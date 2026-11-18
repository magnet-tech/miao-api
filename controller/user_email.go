package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

// maxUserEmailRequestBytes covers a 200 KB body plus 5 MB of images in base64.
const maxUserEmailRequestBytes = 8 << 20

type accountNoticeRequest struct {
	Kind        string `json:"kind"`
	NoticeToken string `json:"notice_token"`
}

// emailRecipient loads the target of an administrator email: the operator
// needs the send permission and a role above the target's (root above all).
func emailRecipient(c *gin.Context) (*model.User, string, bool) {
	if !authz.Can(c.GetInt("id"), c.GetInt("role"), authz.UserEmailSend) {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("Unauthorized, insufficient privileges"))
		return nil, "", false
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return nil, "", false
	}
	user, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiErrorT(c, "User does not exist")
		return nil, "", false
	}
	if !canManageTargetRole(c.GetInt("role"), user.Role) {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to access users of same or higher level"))
		return nil, "", false
	}
	address, err := service.ValidEmailRecipient(user)
	if err != nil {
		common.ApiError(c, err)
		return nil, "", false
	}
	return user, address, true
}

func deliverUserEmail(c *gin.Context, user *model.User, address, kind string, email *service.PreparedEmail) {
	err := common.SendEmailWithImages(email.Subject, address, email.HTML, email.Images)
	success := err == nil
	model.RecordUserEmail(model.UserEmailLog{
		UserId: user.Id, OperatorId: c.GetInt("id"), OperatorUsername: c.GetString("username"),
		Kind: kind, Subject: email.Subject, Success: success,
	})
	recordManageAuditFor(c, user.Id, "user.email_send", map[string]any{
		"username":    user.Username,
		"id":          user.Id,
		"kind":        kind,
		"success":     success,
		"image_count": len(email.Images),
	})
	if err != nil {
		common.SysError(common.LogText("failed to send email to user %d: %s", user.Id, err.Error()))
		common.ApiErrorT(c, "Email delivery failed. Check the email settings before retrying.")
		return
	}
	common.ApiSuccessT(c, "Email sent.", nil)
}

// SendUserEmail sends an administrator's email after step-up verification
// bound to the recipient. POST /api/user/:id/email
func SendUserEmail(c *gin.Context) {
	user, address, ok := emailRecipient(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUserEmailRequestBytes)
	var draft service.UserEmailDraft
	if err := common.DecodeJson(c.Request.Body, &draft); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	email, err := service.PrepareUserEmail(draft)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if requireAdminUserProof(c, service.VerificationScopeAdminUserEmail, service.AdminUserEmailContext{UserID: user.Id, Email: address}) == nil {
		return
	}
	deliverUserEmail(c, user, address, model.UserEmailKindCustom, email)
}

// SendAccountNotice sends the suspension or restore notice with the one-time
// grant the verified disable or enable returned. POST /api/user/:id/notice
func SendAccountNotice(c *gin.Context) {
	user, address, ok := emailRecipient(c)
	if !ok {
		return
	}
	var request accountNoticeRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := model.ConsumeAccountNoticeGrant(request.NoticeToken, c.GetInt("id"), c.GetString("session_id"), user.Id, request.Kind); err != nil {
		common.ApiError(c, err)
		return
	}
	template, err := service.EffectiveAccountNoticeTemplates(service.AccountMailLanguage(c, user.Id)).For(request.Kind)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	email, err := service.RenderAccountNotice(template, user)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	deliverUserEmail(c, user, address, request.Kind, email)
}

// SendSelfTestEmail sends a draft to the operator's own address, so no step-up
// is needed. POST /api/user/self/email/test
func SendSelfTestEmail(c *gin.Context) {
	if !authz.Can(c.GetInt("id"), c.GetInt("role"), authz.UserEmailSend) {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("Unauthorized, insufficient privileges"))
		return
	}
	operator, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	address, err := service.ValidEmailRecipient(operator)
	if err != nil {
		common.ApiErrorT(c, "Bind an email address to your account to receive test emails.")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUserEmailRequestBytes)
	var draft service.UserEmailDraft
	if err := common.DecodeJson(c.Request.Body, &draft); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	email, err := service.PrepareUserEmail(draft)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := common.SendEmailWithImages(email.Subject, address, email.HTML, email.Images); err != nil {
		common.SysError(common.LogText("failed to send test email to user %d: %s", operator.Id, err.Error()))
		common.ApiErrorT(c, "Email delivery failed. Check the email settings before retrying.")
		return
	}
	common.ApiSuccess(c, gin.H{"email": address})
}

// GetUserEmailHistory lists the latest emails sent to a user.
// GET /api/user/:id/email/history
func GetUserEmailHistory(c *gin.Context) {
	if !authz.Can(c.GetInt("id"), c.GetInt("role"), authz.UserEmailSend) {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("Unauthorized, insufficient privileges"))
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	user, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiErrorT(c, "User does not exist")
		return
	}
	if !canManageTargetRole(c.GetInt("role"), user.Role) {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to access users of same or higher level"))
		return
	}
	entries, err := model.GetUserEmailHistory(user.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, entries)
}

// GetEmailTemplates returns the account notice templates. Administrators read
// them to preview and send; root edits them. GET /api/email_templates
func GetEmailTemplates(c *gin.Context) {
	lang := service.AccountMailLanguage(c, c.GetInt("id"))
	_, saved := service.SavedAccountNoticeTemplates()
	site := service.EmailSiteValues()
	common.ApiSuccess(c, gin.H{
		"templates": service.EffectiveAccountNoticeTemplates(lang),
		"defaults":  service.DefaultAccountNoticeTemplates(lang),
		"saved":     saved,
		"variables": service.EmailTemplateVariables,
		"site_name": site["site_name"],
		"site_url":  site["site_url"],
	})
}

// UpdateEmailTemplates saves root's templates. PUT /api/email_templates
func UpdateEmailTemplates(c *gin.Context) {
	var request struct {
		Templates service.AccountNoticeTemplates `json:"templates"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	saved, err := service.SaveAccountNoticeTemplates(request.Templates)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, 0, "option.email_templates_update", map[string]any{})
	common.ApiSuccess(c, gin.H{"templates": saved})
}
