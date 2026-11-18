package controller

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

// An administrator manages another user's API keys through
// /api/user/:id/tokens with the ordinary token handlers, which then work on
// that user's keys. Masked keys are readable with user_token:read. Changes
// need user_token:manage and a management session: one step-up verification
// that lasts 30 minutes for this operator session and this user. Full keys
// need user_token:secret_view and a verification for exactly those keys,
// every time.

const (
	tokenOwnerContextKey         = "token_owner_id"
	managedTokenSessionPurpose   = "admin_token_session"
	managedTokenSessionHeader    = "X-Key-Management-Session"
	managedTokenSessionLifetime  = 30 * time.Minute
	managedTokenActionManage     = "manage"
	managedTokenActionRevealKeys = "reveal"
)

type managedTokenSession struct {
	UserID        int                          `json:"user_id"`
	Authorization *model.AuthFlowAuthorization `json:"authorization"`
}

// tokenOwnerID is whose API keys a token handler works on: the managed user
// under /api/user/:id/tokens, otherwise the signed-in user.
func tokenOwnerID(c *gin.Context) int {
	if owner := c.GetInt(tokenOwnerContextKey); owner > 0 {
		return owner
	}
	return c.GetInt("id")
}

// tokenIDParam is the key ID in the path: :token_id under a managed user,
// :id under /api/token.
func tokenIDParam(c *gin.Context) string {
	if _, managed := c.Get(tokenOwnerContextKey); managed {
		return c.Param("token_id")
	}
	return c.Param("id")
}

// ManagedTokenOwner admits an operator holding permission who may manage the
// :id user, and points the token handlers at that user's keys.
func ManagedTokenOwner(permission authz.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := managedUser(c, permission)
		if !ok {
			c.Abort()
			return
		}
		c.Set(tokenOwnerContextKey, user.Id)
		c.Next()
	}
}

func managedTokenSessionInvalid(c *gin.Context) {
	c.Set("security_error_code", "SECURITY_PROOF_INVALID")
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "code": "SECURITY_PROOF_INVALID", "message": "Security verification is no longer valid. Please verify again."})
}

func managedTokenOperation(userID int, action string, tokenIDs []int) service.VerificationOperation {
	context, _ := common.Marshal(service.AdminUserTokensContext{UserID: userID, Action: action, TokenIDs: tokenIDs})
	return service.VerificationOperation{Scope: service.VerificationScopeAdminUserTokens, Context: context}
}

// StartManagedTokenSession spends a "manage" proof for the user and returns
// a session token for the changes of the next 30 minutes.
func StartManagedTokenSession(c *gin.Context) {
	ownerID := tokenOwnerID(c)
	authorization := requireAdminUserProof(c, service.VerificationScopeAdminUserTokens, service.AdminUserTokensContext{UserID: ownerID, Action: managedTokenActionManage})
	if authorization == nil {
		return
	}
	payload, err := common.Marshal(managedTokenSession{UserID: ownerID, Authorization: authorization})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, flow, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose: managedTokenSessionPurpose, UserId: authorization.UserID, SessionId: authorization.SessionID,
		Payload: string(payload), ExpiresAt: time.Now().Add(managedTokenSessionLifetime),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, ownerID, "user.token_session_start", map[string]any{"target_user_id": ownerID, "method": authorization.Method})
	common.ApiSuccess(c, gin.H{"management_token": token, "expires_at": flow.ExpiresAt.Unix()})
}

// EndManagedTokenSession ends the operator's session early.
func EndManagedTokenSession(c *gin.Context) {
	identity, ok := middleware.GetStepUpIdentity(c)
	token := strings.TrimSpace(c.GetHeader(managedTokenSessionHeader))
	if ok && token != "" {
		_, _ = model.ConsumeAuthFlow(token, model.AuthFlowMatch{Purpose: managedTokenSessionPurpose, UserId: identity.UserID, SessionId: identity.SessionID})
	}
	common.ApiSuccess(c, nil)
}

// RequireManagedTokenSession lets a change through only with a live session
// that this operator session opened for this user, while the verification
// method behind it is still allowed.
func RequireManagedTokenSession(c *gin.Context) {
	identity, ok := middleware.GetStepUpIdentity(c)
	if !ok {
		managedTokenSessionInvalid(c)
		return
	}
	flow, err := model.GetAuthFlow(strings.TrimSpace(c.GetHeader(managedTokenSessionHeader)), model.AuthFlowMatch{Purpose: managedTokenSessionPurpose, UserId: identity.UserID, SessionId: identity.SessionID})
	if err != nil {
		managedTokenSessionInvalid(c)
		return
	}
	var session managedTokenSession
	ownerID := tokenOwnerID(c)
	if common.UnmarshalJsonStr(flow.Payload, &session) != nil || session.UserID != ownerID ||
		service.ValidateFlowAuthorization(identity, managedTokenOperation(ownerID, managedTokenActionManage, nil), session.Authorization) != nil {
		managedTokenSessionInvalid(c)
		return
	}
	c.Next()
}

// RevealManagedTokenKey shows one full key after a "reveal" proof for it.
func RevealManagedTokenKey(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("token_id"))
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if requireAdminUserProof(c, service.VerificationScopeAdminUserTokens, service.AdminUserTokensContext{UserID: tokenOwnerID(c), Action: managedTokenActionRevealKeys, TokenIDs: []int{id}}) == nil {
		return
	}
	GetTokenKey(c)
}

// RevealManagedTokenKeys shows several full keys after a "reveal" proof for
// exactly those keys.
func RevealManagedTokenKeys(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var request struct {
		Ids []int `json:"ids"`
	}
	if common.Unmarshal(body, &request) != nil || len(request.Ids) == 0 || len(request.Ids) > service.MaxRevealedTokenKeys {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if requireAdminUserProof(c, service.VerificationScopeAdminUserTokens, service.AdminUserTokensContext{UserID: tokenOwnerID(c), Action: managedTokenActionRevealKeys, TokenIDs: request.Ids}) == nil {
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	GetTokenKeysBatch(c)
}
