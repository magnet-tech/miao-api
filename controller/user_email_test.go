package controller

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http"
	"net/mail"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A 1x1 PNG.
const testPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

type receivedMail struct {
	Subject string
	HTML    string
	CIDs    []string
}

// lastMail decodes the latest message delivered to receiver: subject, HTML
// body and the Content-IDs of inline images.
func lastMail(t *testing.T, mailbox *securityMailbox, receiver string) receivedMail {
	t.Helper()
	mailbox.mutex.Lock()
	messages := mailbox.mail[receiver]
	mailbox.mutex.Unlock()
	require.NotEmpty(t, messages, "no email delivered to %s", receiver)
	message, err := mail.ReadMessage(strings.NewReader(messages[len(messages)-1]))
	require.NoError(t, err)
	subject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
	require.NoError(t, err)
	received := receivedMail{Subject: subject}
	mediaType, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	require.NoError(t, err)
	if !strings.HasPrefix(mediaType, "multipart/") {
		body, err := io.ReadAll(message.Body)
		require.NoError(t, err)
		received.HTML = strings.TrimRight(string(body), "\r\n")
		return received
	}
	reader := multipart.NewReader(message.Body, params["boundary"])
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if cid := part.Header.Get("Content-ID"); cid != "" {
			received.CIDs = append(received.CIDs, strings.Trim(cid, "<>"))
			continue
		}
		body, err := io.ReadAll(quotedprintable.NewReader(part))
		require.NoError(t, err)
		received.HTML = strings.TrimRight(string(body), "\r\n")
	}
	return received
}

func setupUserEmailTest(t *testing.T) (*model.User, service.AuthIdentity, *model.User, *securityMailbox) {
	t.Helper()
	operator, identity, target := setupAdminUserTest(t)
	configureEmailDelivery(t, map[string]string{"EmailProvider": "smtp"})
	mailbox := newSecurityMailbox(t)
	require.NoError(t, model.DB.AutoMigrate(&model.UserEmailLog{}, &model.CasbinRule{}, &model.AuthzRole{}))
	common.IsMasterNode = true
	require.NoError(t, authz.Init(model.DB))
	require.NoError(t, model.DB.Model(target).Updates(map[string]any{"email": "managed@example.com", "display_name": "<b>Managed</b>"}).Error)
	target.Email = "managed@example.com"
	return operator, identity, target, mailbox
}

func emailProof(t *testing.T, identity service.AuthIdentity, userID int, email string) string {
	t.Helper()
	context, err := common.Marshal(service.AdminUserEmailContext{UserID: userID, Email: email})
	require.NoError(t, err)
	return issueSecurityEnrollmentProof(t, identity, service.VerificationOperation{Scope: service.VerificationScopeAdminUserEmail, Context: context}, service.VerificationMethodPassword)
}

func idParams(id int) gin.Params {
	return gin.Params{{Key: "id", Value: fmt.Sprint(id)}}
}

func TestPrepareUserEmailKeepsOnlySafeFormatting(t *testing.T) {
	email, err := service.PrepareUserEmail(service.UserEmailDraft{
		Subject: "  Hello  ",
		HTML: `<h2 onclick="steal()">Title</h2><script>alert(1)</script><p style="color:red; text-align: center">Body <a href="javascript:alert(1)">bad</a> <a href="https://example.com/x">good</a></p>` +
			`<iframe src="https://evil.example"></iframe><custom>kept text</custom><img src="cid:logo"><img src="cid:missing"><img src="data:image/png;base64,AAAA">`,
		Images: []common.InlineEmailImage{
			{CID: "logo", ContentType: "image/png", Content: testPNG},
			{CID: "unused", ContentType: "image/png", Content: testPNG},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "Hello", email.Subject)
	assert.Equal(t, `<h2>Title</h2><p style="text-align: center">Body <a>bad</a> <a href="https://example.com/x" rel="noopener noreferrer">good</a></p>kept text<img src="cid:logo">`, email.HTML)
	require.Len(t, email.Images, 1, "images the body does not use are not sent")
	assert.Equal(t, "logo", email.Images[0].CID)

	for name, draft := range map[string]service.UserEmailDraft{
		"header injection":   {Subject: "Hi\r\nBcc: x@example.com", HTML: "<p>x</p>"},
		"empty subject":      {Subject: " ", HTML: "<p>x</p>"},
		"empty body":         {Subject: "Hi", HTML: "<script>x</script><p> </p>"},
		"type mismatch":      {Subject: "Hi", HTML: `<img src="cid:a">`, Images: []common.InlineEmailImage{{CID: "a", ContentType: "image/jpeg", Content: testPNG}}},
		"not an image":       {Subject: "Hi", HTML: `<img src="cid:a">`, Images: []common.InlineEmailImage{{CID: "a", ContentType: "image/png", Content: base64.StdEncoding.EncodeToString([]byte("<svg/>"))}}},
		"invalid content id": {Subject: "Hi", HTML: `<p>x</p>`, Images: []common.InlineEmailImage{{CID: "a b", ContentType: "image/png", Content: testPNG}}},
		"too many images": {Subject: "Hi", HTML: `<p>x</p>`, Images: func() []common.InlineEmailImage {
			images := make([]common.InlineEmailImage, 6)
			for i := range images {
				images[i] = common.InlineEmailImage{CID: fmt.Sprintf("i%d", i), ContentType: "image/png", Content: testPNG}
			}
			return images
		}()},
	} {
		_, err := service.PrepareUserEmail(draft)
		assert.Error(t, err, name)
	}
}

func TestSendUserEmailRequiresProofForTheRecipient(t *testing.T) {
	_, identity, target, mailbox := setupUserEmailTest(t)
	body := fmt.Sprintf(`{"subject":"Notice","html":"<p>Hello <img src=\"cid:logo\"></p>","images":[{"cid":"logo","content_type":"image/png","content":"%s"}]}`, testPNG)

	missing := adminUserRequest(http.MethodPost, "/api/user/:id/email", body, "", identity, common.RoleRootUser, idParams(target.Id), SendUserEmail)
	assert.Contains(t, missing.Body.String(), `"success":false`)
	otherAddress := adminUserRequest(http.MethodPost, "/api/user/:id/email", body, emailProof(t, identity, target.Id, "other@example.com"), identity, common.RoleRootUser, idParams(target.Id), SendUserEmail)
	assert.Contains(t, otherAddress.Body.String(), `"success":false`)
	mailbox.mutex.Lock()
	assert.Empty(t, mailbox.mail[target.Email])
	mailbox.mutex.Unlock()

	proof := emailProof(t, identity, target.Id, target.Email)
	sent := adminUserRequest(http.MethodPost, "/api/user/:id/email", body, proof, identity, common.RoleRootUser, idParams(target.Id), SendUserEmail)
	require.Contains(t, sent.Body.String(), `"success":true`, sent.Body.String())
	received := lastMail(t, mailbox, target.Email)
	assert.Equal(t, "Notice", received.Subject)
	assert.Equal(t, `<p>Hello <img src="cid:logo"></p>`, received.HTML)
	assert.Equal(t, []string{"logo"}, received.CIDs)

	replayed := adminUserRequest(http.MethodPost, "/api/user/:id/email", body, proof, identity, common.RoleRootUser, idParams(target.Id), SendUserEmail)
	assert.Contains(t, replayed.Body.String(), `"success":false`, "a proof sends once")

	history, err := model.GetUserEmailHistory(target.Id)
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, model.UserEmailKindCustom, history[0].Kind)
	assert.Equal(t, "Notice", history[0].Subject)
	assert.True(t, history[0].Success)
	var audits []model.AuditLog
	require.NoError(t, model.LOG_DB.Where("action = ?", "user.email_send").Find(&audits).Error)
	require.Len(t, audits, 1)
	assert.NotContains(t, audits[0].Content+fmt.Sprint(audits[0].Other), "Hello", "the body is not audited")
}

func TestSendUserEmailFollowsRoleAndPermission(t *testing.T) {
	operator, identity, target, _ := setupUserEmailTest(t)
	body := `{"subject":"Notice","html":"<p>Hello</p>"}`
	require.NoError(t, model.DB.Model(operator).Update("role", common.RoleAdminUser).Error)
	require.NoError(t, model.PublishUserAuthCache(operator.Id))

	peer := &model.User{Username: "peer-admin", Password: "x", Email: "peer@example.com", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "peer-aff"}
	require.NoError(t, model.DB.Create(peer).Error)
	toPeer := adminUserRequest(http.MethodPost, "/api/user/:id/email", body, emailProof(t, identity, peer.Id, peer.Email), identity, common.RoleAdminUser, idParams(peer.Id), SendUserEmail)
	assert.Equal(t, http.StatusForbidden, toPeer.Code, "administrators email common users only")

	require.NoError(t, authz.SetUserPermissions(operator.Id, authz.PermissionsMap{authz.ResourceUserEmail: {authz.ActionSend: false}}))
	denied := adminUserRequest(http.MethodPost, "/api/user/:id/email", body, emailProof(t, identity, target.Id, target.Email), identity, common.RoleAdminUser, idParams(target.Id), SendUserEmail)
	assert.Equal(t, http.StatusForbidden, denied.Code)
}

func TestAccountNoticeUsesTheVerifiedManageGrantOnce(t *testing.T) {
	_, identity, target, mailbox := setupUserEmailTest(t)
	manageProof := issueSecurityEnrollmentProof(t, identity, service.VerificationOperation{
		Scope:   service.VerificationScopeAdminUserManage,
		Context: []byte(fmt.Sprintf(`{"user_id":%d,"action":"disable"}`, target.Id)),
	}, service.VerificationMethodPassword)
	disabled := adminUserRequest(http.MethodPost, "/api/user/manage", fmt.Sprintf(`{"id":%d,"action":"disable"}`, target.Id), manageProof, identity, common.RoleRootUser, nil, ManageUser)
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			NoticeToken string `json:"notice_token"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(disabled.Body.Bytes(), &result), disabled.Body.String())
	require.True(t, result.Success)
	require.NotEmpty(t, result.Data.NoticeToken)

	notice := func(kind, token string) string {
		body := fmt.Sprintf(`{"kind":"%s","notice_token":"%s"}`, kind, token)
		return adminUserRequest(http.MethodPost, "/api/user/:id/notice", body, "", identity, common.RoleRootUser, idParams(target.Id), SendAccountNotice).Body.String()
	}
	assert.Contains(t, notice(model.UserEmailKindRestored, result.Data.NoticeToken), `"success":false`, "the grant names one kind of notice")
	assert.Contains(t, notice(model.UserEmailKindSuspended, result.Data.NoticeToken), `"success":false`, "a mismatched attempt spends the grant")

	// A fresh grant sends the suspension notice from the saved template.
	_, err := service.SaveAccountNoticeTemplates(service.AccountNoticeTemplates{
		Suspended: service.EmailTemplate{Subject: "{{site_name}}: account {{username}} suspended", HTML: `<p>Hi {{display_name}}</p><script>x</script>`},
		Restored:  service.EmailTemplate{Subject: "Restored", HTML: "<p>Back</p>"},
	})
	require.NoError(t, err)
	token, _, err := model.CreateAccountNoticeGrant(identity.UserID, identity.SessionID, target.Id, model.UserEmailKindSuspended)
	require.NoError(t, err)
	assert.Contains(t, notice(model.UserEmailKindSuspended, token), `"success":true`)
	received := lastMail(t, mailbox, target.Email)
	assert.Equal(t, common.SystemName+": account managed-user suspended", received.Subject)
	assert.Equal(t, "<p>Hi &lt;b&gt;Managed&lt;/b&gt;</p>", received.HTML, "values are escaped and the template is sanitized")
	assert.Contains(t, notice(model.UserEmailKindSuspended, token), `"success":false`, "a grant works once")

	otherSession, _, err := model.CreateAccountNoticeGrant(identity.UserID, "another-session", target.Id, model.UserEmailKindSuspended)
	require.NoError(t, err)
	assert.Contains(t, notice(model.UserEmailKindSuspended, otherSession), `"success":false`, "a grant belongs to the operator session")
}
