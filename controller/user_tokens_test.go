package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// managedTokenRouter wires the /api/user/:id/tokens routes as the API router
// does, behind an operator session that AdminAuth would have established.
func managedTokenRouter(identity service.AuthIdentity, role *int) *gin.Engine {
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("id", identity.UserID)
		c.Set("username", "operator")
		c.Set("role", *role)
		c.Set("session_id", identity.SessionID)
		c.Set("auth_version", identity.UserAuthVersion)
		c.Set("session_version", identity.SessionVersion)
	})
	tokens := engine.Group("/api/user/:id/tokens", middleware.TokenOperationAudit())
	read := ManagedTokenOwner(authz.UserTokenRead)
	manage := ManagedTokenOwner(authz.UserTokenManage)
	reveal := ManagedTokenOwner(authz.UserTokenSecretView)
	tokens.GET("/", read, GetAllTokens)
	tokens.POST("/management-session", manage, StartManagedTokenSession)
	tokens.DELETE("/management-session", EndManagedTokenSession)
	tokens.POST("/", manage, RequireManagedTokenSession, AddToken)
	tokens.DELETE("/:token_id", manage, RequireManagedTokenSession, DeleteToken)
	tokens.POST("/:token_id/key", reveal, RevealManagedTokenKey)
	tokens.POST("/batch/keys", reveal, RevealManagedTokenKeys)
	return engine
}

type managedTokenCall struct {
	method, path, body, session, proof string
}

func (call managedTokenCall) send(engine *gin.Engine) (int, map[string]any) {
	request := httptest.NewRequest(call.method, call.path, strings.NewReader(call.body))
	request.Header.Set("Content-Type", "application/json")
	if call.session != "" {
		request.Header.Set("X-Key-Management-Session", call.session)
	}
	if call.proof != "" {
		request.Header.Set("X-Security-Proof", call.proof)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	var body map[string]any
	_ = common.Unmarshal(recorder.Body.Bytes(), &body)
	return recorder.Code, body
}

func tokensProof(t *testing.T, identity service.AuthIdentity, userID int, action string, tokenIDs ...int) string {
	t.Helper()
	context := service.AdminUserTokensContext{UserID: userID, Action: action, TokenIDs: tokenIDs}
	payload, err := common.Marshal(context)
	require.NoError(t, err)
	return issueSecurityEnrollmentProof(t, identity, service.VerificationOperation{Scope: service.VerificationScopeAdminUserTokens, Context: payload}, service.VerificationMethodPassword)
}

func setupManagedTokenTest(t *testing.T) (*model.User, service.AuthIdentity, *model.User, *gin.Engine, *int) {
	t.Helper()
	operator, identity, target := setupAdminUserTest(t)
	initDialectColumns(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}, &model.CasbinRule{}, &model.AuthzRole{}))
	common.IsMasterNode = true
	require.NoError(t, authz.Init(model.DB))
	role := common.RoleRootUser
	return operator, identity, target, managedTokenRouter(identity, &role), &role
}

func startManagedTokenSession(t *testing.T, engine *gin.Engine, identity service.AuthIdentity, userID int) string {
	t.Helper()
	code, body := managedTokenCall{method: http.MethodPost, path: fmt.Sprintf("/api/user/%d/tokens/management-session", userID), proof: tokensProof(t, identity, userID, "manage")}.send(engine)
	require.Equal(t, http.StatusOK, code, body)
	require.Equal(t, true, body["success"], body)
	return body["data"].(map[string]any)["management_token"].(string)
}

func TestManagedTokenChangesNeedASessionForThatUser(t *testing.T) {
	operator, identity, target, engine, _ := setupManagedTokenTest(t)
	other := &model.User{Username: "other-user", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "other-aff"}
	require.NoError(t, model.DB.Create(other).Error)
	create := func(userID int, session string) (int, map[string]any) {
		return managedTokenCall{method: http.MethodPost, path: fmt.Sprintf("/api/user/%d/tokens/", userID), body: `{"name":"managed","unlimited_quota":true}`, session: session}.send(engine)
	}
	countKeys := func(userID int) int64 {
		var count int64
		require.NoError(t, model.DB.Model(&model.Token{}).Where("user_id = ?", userID).Count(&count).Error)
		return count
	}

	code, body := create(target.Id, "")
	assert.Equal(t, http.StatusForbidden, code)
	assert.Equal(t, "SECURITY_PROOF_INVALID", body["code"])

	code, _ = managedTokenCall{method: http.MethodPost, path: fmt.Sprintf("/api/user/%d/tokens/management-session", target.Id), proof: tokensProof(t, identity, other.Id, "manage")}.send(engine)
	assert.Equal(t, http.StatusForbidden, code, "a proof for another user does not open a session")

	session := startManagedTokenSession(t, engine, identity, target.Id)
	code, body = create(target.Id, session)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, true, body["success"], body)
	assert.Equal(t, int64(1), countKeys(target.Id), "the key belongs to the managed user")
	assert.Zero(t, countKeys(operator.Id))

	code, _ = create(other.Id, session)
	assert.Equal(t, http.StatusForbidden, code, "a session covers only the user it was opened for")
	assert.Zero(t, countKeys(other.Id))

	otherSession := service.AuthIdentity{UserID: identity.UserID, SessionID: "another-session", UserAuthVersion: identity.UserAuthVersion, SessionVersion: identity.SessionVersion}
	stolenRole := common.RoleRootUser
	code, _ = managedTokenCall{method: http.MethodPost, path: fmt.Sprintf("/api/user/%d/tokens/", target.Id), body: `{"name":"x"}`, session: session}.send(managedTokenRouter(otherSession, &stolenRole))
	assert.Equal(t, http.StatusForbidden, code, "a session belongs to the operator session that opened it")

	code, _ = managedTokenCall{method: http.MethodDelete, path: fmt.Sprintf("/api/user/%d/tokens/management-session", target.Id), session: session}.send(engine)
	require.Equal(t, http.StatusOK, code)
	code, _ = create(target.Id, session)
	assert.Equal(t, http.StatusForbidden, code, "an ended session no longer works")

	var audits []model.AuditLog
	require.NoError(t, model.LOG_DB.Where("action = ?", "user_token.create").Find(&audits).Error)
	require.NotEmpty(t, audits)
	require.NotNil(t, audits[0].Other.Op)
	assert.JSONEq(t, fmt.Sprint(target.Id), string(audits[0].Other.Op.Params["target_user_id"].(json.RawMessage)))
}

func TestManagedTokenKeysAreShownOncePerVerification(t *testing.T) {
	_, identity, target, engine, role := setupManagedTokenTest(t)
	secret, err := common.GenerateKey()
	require.NoError(t, err)
	key := model.Token{UserId: target.Id, Name: "secret", Key: secret, Status: common.TokenStatusEnabled, UnlimitedQuota: true}
	require.NoError(t, model.DB.Create(&key).Error)
	path := fmt.Sprintf("/api/user/%d/tokens/%d/key", target.Id, key.Id)

	code, body := managedTokenCall{method: http.MethodPost, path: path}.send(engine)
	assert.Equal(t, http.StatusForbidden, code)
	assert.Equal(t, "SECURITY_PROOF_REQUIRED", body["code"])
	code, _ = managedTokenCall{method: http.MethodPost, path: path, proof: tokensProof(t, identity, target.Id, "reveal", key.Id+1)}.send(engine)
	assert.Equal(t, http.StatusForbidden, code, "a proof names the keys it reveals")
	code, _ = managedTokenCall{method: http.MethodPost, path: path, proof: tokensProof(t, identity, target.Id, "manage")}.send(engine)
	assert.Equal(t, http.StatusForbidden, code, "a management proof does not reveal keys")

	proof := tokensProof(t, identity, target.Id, "reveal", key.Id)
	code, body = managedTokenCall{method: http.MethodPost, path: path, proof: proof}.send(engine)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, key.GetFullKey(), body["data"].(map[string]any)["key"])
	code, _ = managedTokenCall{method: http.MethodPost, path: path, proof: proof}.send(engine)
	assert.Equal(t, http.StatusForbidden, code, "a reveal proof works once")

	batch := managedTokenCall{method: http.MethodPost, path: fmt.Sprintf("/api/user/%d/tokens/batch/keys", target.Id), body: fmt.Sprintf(`{"ids":[%d]}`, key.Id), proof: tokensProof(t, identity, target.Id, "reveal", key.Id)}
	code, body = batch.send(engine)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, true, body["success"], body)

	// Administrators cannot reveal keys unless root grants it.
	*role = common.RoleAdminUser
	code, _ = managedTokenCall{method: http.MethodPost, path: path, proof: tokensProof(t, identity, target.Id, "reveal", key.Id)}.send(engine)
	assert.Equal(t, http.StatusForbidden, code)
}

func TestManagedTokensFollowRoleAndPermission(t *testing.T) {
	operator, _, target, engine, role := setupManagedTokenTest(t)
	*role = common.RoleAdminUser
	require.NoError(t, model.DB.Model(operator).Update("role", common.RoleAdminUser).Error)
	require.NoError(t, model.PublishUserAuthCache(operator.Id))
	peer := &model.User{Username: "peer-admin", Password: "x", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "peer-aff"}
	require.NoError(t, model.DB.Create(peer).Error)

	list := func(userID int) int {
		code, _ := managedTokenCall{method: http.MethodGet, path: fmt.Sprintf("/api/user/%d/tokens/", userID)}.send(engine)
		return code
	}
	assert.Equal(t, http.StatusOK, list(target.Id))
	assert.Equal(t, http.StatusForbidden, list(peer.Id), "administrators manage the keys of common users only")
	require.NoError(t, authz.SetUserPermissions(operator.Id, authz.PermissionsMap{authz.ResourceUserToken: {authz.ActionRead: false}}))
	assert.Equal(t, http.StatusForbidden, list(target.Id))
}
