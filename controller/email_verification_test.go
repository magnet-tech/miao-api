package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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

// initDialectColumns prepares the dialect-specific column quoting that
// TokenAuth's token lookup uses, through the same model.InitDB path as the
// server, on a throwaway database; the fixture's connections are restored.
func initDialectColumns(t *testing.T) {
	t.Helper()
	dialect := os.Getenv("TEST_SECURITY_DIALECT")
	if dialect == "" {
		dialect = "sqlite"
	}
	startupDB, dsn := newAuditTestDatabase(t, dialect, os.Getenv("TEST_"+strings.ToUpper(dialect)+"_DSN"))
	if connection, err := startupDB.DB(); err == nil {
		_ = connection.Close()
	}
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousSQLite, previousMaster := common.SQLitePath, common.IsMasterNode
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	common.IsMasterNode = false
	if dialect == "sqlite" {
		common.SQLitePath, dsn = dsn, "local"
	}
	t.Setenv("SQL_DSN", dsn)
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitDB())
	if connection, err := model.DB.DB(); err == nil {
		_ = connection.Close()
	}
	model.DB, model.LOG_DB = previousDB, previousLogDB
	common.SQLitePath, common.IsMasterNode = previousSQLite, previousMaster
	common.SetDatabaseTypes(previousMain, previousLog)
}

// emailGateStatus runs one request through a gate and answers 204 when the
// gate lets it through. Token gates read the key; the playground gate reads
// the session user id that UserAuth would have set.
func emailGateStatus(t *testing.T, userID int, gate gin.HandlerFunc, key string) int {
	t.Helper()
	router := gin.New()
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		if key == "" {
			c.Set("id", userID)
		}
		c.Next()
	}, gate, func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}"))
	if key != "" {
		request.Header.Set("Authorization", "Bearer sk-"+key)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response.Code
}

func adminUserUpdate(t *testing.T, operatorID, operatorRole int, body string, handler gin.HandlerFunc) securityEnrollmentResponse {
	t.Helper()
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/user/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", operatorID)
	c.Set("role", operatorRole)
	handler(c)
	var result securityEnrollmentResponse
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result), response.Body.String())
	return result
}

func emailVerificationCall(t *testing.T, identity service.AuthIdentity, path, body string, handler gin.HandlerFunc) (securityEnrollmentResponse, map[string]any) {
	t.Helper()
	response := securityEnrollmentRequest(http.MethodPost, path, body, "", identity, handler)
	var result securityEnrollmentResponse
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result), response.Body.String())
	data := map[string]any{}
	if len(result.Data) > 0 {
		require.NoError(t, common.Unmarshal(result.Data, &data))
	}
	return result, data
}

func storedEmailState(t *testing.T, userID int) model.User {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("id", "email", "email_verified", "require_verified_email").First(&user, userID).Error)
	return user
}

func TestEmailVerificationRequirementGatesAPIAccess(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	initDialectColumns(t)
	mailbox := newSecurityMailbox(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}, &model.CasbinRule{}, &model.AuthzRole{}))
	common.IsMasterNode = true
	require.NoError(t, authz.Init(model.DB))
	require.NoError(t, model.DB.Model(user).Update("email", "owner@example.com").Error)
	token := &model.Token{UserId: user.Id, Key: strings.Repeat("b", 48), Status: common.TokenStatusEnabled, UnlimitedQuota: true, ExpiredTime: -1}
	require.NoError(t, model.DB.Create(token).Error)
	gates := map[string]gin.HandlerFunc{
		"relay":      middleware.TokenAuth(),
		"read-only":  middleware.TokenAuthReadOnly(),
		"playground": middleware.VerifiedEmailRequired(),
	}
	keyFor := func(name string) string {
		if name == "playground" {
			return ""
		}
		return token.Key
	}
	for name, gate := range gates {
		assert.Equal(t, http.StatusNoContent, emailGateStatus(t, user.Id, gate, keyFor(name)), "no requirement: %s", name)
	}

	// An administrator turns the requirement on; omitting the field later keeps it.
	update := fmt.Sprintf(`{"id":%d,"username":"%s","display_name":"%s","group":"default","require_verified_email":true}`, user.Id, user.Username, user.Username)
	result := adminUserUpdate(t, 1, common.RoleRootUser, update, UpdateUser)
	require.True(t, result.Success, result.Message)
	assert.True(t, storedEmailState(t, user.Id).RequireVerifiedEmail)
	omitted := fmt.Sprintf(`{"id":%d,"username":"%s","display_name":"renamed","group":"default"}`, user.Id, user.Username)
	require.True(t, adminUserUpdate(t, 1, common.RoleRootUser, omitted, UpdateUser).Success)
	assert.True(t, storedEmailState(t, user.Id).RequireVerifiedEmail, "an omitted field keeps the requirement")
	for name, gate := range gates {
		assert.Equal(t, http.StatusForbidden, emailGateStatus(t, user.Id, gate, keyFor(name)), "required and unverified: %s", name)
	}

	// The user cannot mark the address verified through the profile update.
	forged := model.User{Id: user.Id, DisplayName: "forged", EmailVerified: true}
	require.NoError(t, forged.Update(false))
	assert.False(t, storedEmailState(t, user.Id).EmailVerified)

	// Verify the current address: a wrong code counts, the right one unblocks,
	// and the flow cannot be replayed.
	started, data := emailVerificationCall(t, identity, "/api/user/self/email/verify/start", `{}`, EmailVerificationStart)
	require.True(t, started.Success, started.Message)
	assert.Equal(t, model.EmailVerificationModeCurrent, data["mode"])
	flowToken := data["flow_token"].(string)
	code := mailbox.code(t, "owner@example.com")
	wrong := "000000"
	if wrong == code {
		wrong = "111111"
	}
	rejected, _ := emailVerificationCall(t, identity, "/api/user/self/email/verify", fmt.Sprintf(`{"flow_token":"%s","code":"%s"}`, flowToken, wrong), EmailVerificationFinish)
	assert.False(t, rejected.Success)
	assert.Equal(t, "EMAIL_BINDING_CODE_INVALID", rejected.Code)
	assert.Equal(t, http.StatusForbidden, emailGateStatus(t, user.Id, gates["relay"], token.Key))
	finished, _ := emailVerificationCall(t, identity, "/api/user/self/email/verify", fmt.Sprintf(`{"flow_token":"%s","code":"%s"}`, flowToken, code), EmailVerificationFinish)
	require.True(t, finished.Success, finished.Message)
	assert.True(t, storedEmailState(t, user.Id).EmailVerified)
	for name, gate := range gates {
		assert.Equal(t, http.StatusNoContent, emailGateStatus(t, user.Id, gate, keyFor(name)), "verified: %s", name)
	}
	replayed, _ := emailVerificationCall(t, identity, "/api/user/self/email/verify", fmt.Sprintf(`{"flow_token":"%s","code":"%s"}`, flowToken, code), EmailVerificationFinish)
	assert.False(t, replayed.Success)

	// Clearing the address drops the verification and blocks again.
	require.NoError(t, (&model.User{Id: user.Id}).ClearBinding("email"))
	assert.False(t, storedEmailState(t, user.Id).EmailVerified)
	assert.Equal(t, http.StatusForbidden, emailGateStatus(t, user.Id, gates["relay"], token.Key))

	// A regular email binding marks the new address verified.
	flow := startSecurityEmailBinding(t, identity, "bound@example.com", service.VerificationMethodPassword)
	_, err := service.FinishEmailBinding(identity, flow.FlowToken, mailbox.code(t, "bound@example.com"), "")
	require.NoError(t, err)
	assert.True(t, storedEmailState(t, user.Id).EmailVerified)
	assert.Equal(t, http.StatusNoContent, emailGateStatus(t, user.Id, gates["relay"], token.Key))

	// Root is never blocked, whatever is stored.
	require.NoError(t, model.DB.Model(user).Updates(map[string]any{"role": common.RoleRootUser, "email_verified": false}).Error)
	model.InvalidateEmailVerificationGate(user.Id)
	assert.Equal(t, http.StatusNoContent, emailGateStatus(t, user.Id, gates["playground"], ""))
}

func TestEmailVerificationRequirementNeedsPermission(t *testing.T) {
	user, _ := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.CasbinRule{}, &model.AuthzRole{}))
	common.IsMasterNode = true
	require.NoError(t, authz.Init(model.DB))
	const adminID = 900
	require.NoError(t, authz.SetUserPermissions(adminID, authz.PermissionsMap{
		authz.ResourceUserEmailVerification: {authz.ActionWrite: false},
	}))
	update := fmt.Sprintf(`{"id":%d,"username":"%s","display_name":"%s","group":"default","require_verified_email":true}`, user.Id, user.Username, user.Username)
	result := adminUserUpdate(t, adminID, common.RoleAdminUser, update, UpdateUser)
	assert.False(t, result.Success)
	assert.False(t, storedEmailState(t, user.Id).RequireVerifiedEmail)

	// Other fields still update when the requirement is left alone.
	unchanged := fmt.Sprintf(`{"id":%d,"username":"%s","display_name":"renamed","group":"default","require_verified_email":false}`, user.Id, user.Username)
	assert.True(t, adminUserUpdate(t, adminID, common.RoleAdminUser, unchanged, UpdateUser).Success)
}

func TestEmailVerificationRequiredBindingWithoutStepUp(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	mailbox := newSecurityMailbox(t)
	require.NoError(t, model.DB.Model(user).Update("require_verified_email", true).Error)

	// With a password the account must use the regular binding with step-up.
	withPassword, _ := emailVerificationCall(t, identity, "/api/user/self/email/verify/start", `{"email":"first@example.com"}`, EmailVerificationStart)
	assert.False(t, withPassword.Success)
	assert.Equal(t, "EMAIL_VERIFICATION_STEP_UP_REQUIRED", withPassword.Code)
	assert.Equal(t, "Verify your identity before binding an email address.", withPassword.MessageKey, "the console translates the reason")

	// Without any step-up method the first address can be bound directly.
	require.NoError(t, model.DB.Model(user).Update("password", "").Error)
	taken := &model.User{Username: "taken-owner", Password: "x", Email: "taken@example.com", AffCode: "taken-aff", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, model.DB.Create(taken).Error)
	conflict, _ := emailVerificationCall(t, identity, "/api/user/self/email/verify/start", `{"email":"taken@example.com"}`, EmailVerificationStart)
	assert.False(t, conflict.Success)
	assert.Equal(t, "EMAIL_ALREADY_TAKEN", conflict.Code)

	started, data := emailVerificationCall(t, identity, "/api/user/self/email/verify/start", `{"email":"First@Example.com"}`, EmailVerificationStart)
	require.True(t, started.Success, started.Message)
	assert.Equal(t, model.EmailVerificationModeRequired, data["mode"])
	assert.Equal(t, "first@example.com", data["email"])
	code := mailbox.code(t, "first@example.com")
	finished, _ := emailVerificationCall(t, identity, "/api/user/self/email/verify", fmt.Sprintf(`{"flow_token":"%s","code":"%s"}`, data["flow_token"], code), EmailVerificationFinish)
	require.True(t, finished.Success, finished.Message)
	stored := storedEmailState(t, user.Id)
	assert.Equal(t, "first@example.com", stored.Email)
	assert.True(t, stored.EmailVerified)

	// Once an address exists, the direct binding is closed.
	again, _ := emailVerificationCall(t, identity, "/api/user/self/email/verify/start", `{"email":"second@example.com"}`, EmailVerificationStart)
	assert.False(t, again.Success)
}

func TestEmailVerificationColumnsUpgradeKeepsData(t *testing.T) {
	user, _ := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.Model(user).Updates(map[string]any{"email": "kept@example.com", "quota": 4321}).Error)
	// A database from main already has email_verified; one from refactor has
	// neither column. Both upgrade without losing data.
	require.NoError(t, model.DB.Model(user).Update("email_verified", true).Error)
	require.NoError(t, model.DB.Migrator().DropColumn(&model.User{}, "require_verified_email"))
	for range 2 {
		require.NoError(t, model.DB.AutoMigrate(&model.User{}))
	}
	stored, err := model.GetUserById(user.Id, true)
	require.NoError(t, err)
	assert.True(t, stored.EmailVerified)
	assert.False(t, stored.RequireVerifiedEmail)
	assert.Equal(t, 4321, stored.Quota)

	require.NoError(t, model.DB.Migrator().DropColumn(&model.User{}, "email_verified"))
	require.NoError(t, model.DB.Migrator().DropColumn(&model.User{}, "require_verified_email"))
	for range 2 {
		require.NoError(t, model.DB.AutoMigrate(&model.User{}))
	}
	stored, err = model.GetUserById(user.Id, true)
	require.NoError(t, err)
	assert.False(t, stored.EmailVerified)
	assert.False(t, stored.RequireVerifiedEmail)
	assert.Equal(t, "kept@example.com", stored.Email)
	assert.True(t, model.DB.Migrator().HasIndex(&model.User{}, "idx_users_username"))
}
