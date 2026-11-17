package service

import (
	"encoding/base64"
	"errors"
	"html"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Emails administrators write to users, and the suspension and restore
// notices offered after a user is disabled or enabled.

const (
	maxUserEmailImages       = 5
	maxUserEmailImageBytes   = 5 << 20
	maxUserEmailHTMLBytes    = 200_000
	maxUserEmailSubjectRunes = 200

	AccountNoticeTemplatesOption = "AccountNoticeEmailTemplates"
)

var (
	ErrUserEmailSubject       = errors.New("Subject must be 1 to 200 characters.")
	ErrUserEmailBodyEmpty     = errors.New("The email body is empty.")
	ErrUserEmailBodyTooLong   = errors.New("The email body is too long.")
	ErrUserEmailImageCount    = errors.New("An email can contain at most 5 images.")
	ErrUserEmailImageSize     = errors.New("Images can be at most 5 MB in total.")
	ErrUserEmailImageInvalid  = errors.New("Only PNG, JPEG, GIF and WebP images can be inserted.")
	ErrUserEmailRecipient     = errors.New("This user has no valid email address.")
	ErrUserEmailNoticeKind    = errors.New("Unknown notice type.")
	ErrUserEmailTemplateEmpty = errors.New("Each notice template needs a subject and a body.")

	emailImageCID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	templateVar   = regexp.MustCompile(`\{\{\s*([a-z_]+)\s*\}\}`)

	emailImageTypes = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

	// Formatting the console editor produces; anything else is unwrapped.
	emailAllowedTags = map[atom.Atom]bool{
		atom.P: true, atom.Div: true, atom.Span: true, atom.Br: true,
		atom.B: true, atom.Strong: true, atom.I: true, atom.Em: true, atom.U: true, atom.S: true,
		atom.H1: true, atom.H2: true, atom.H3: true, atom.Blockquote: true,
		atom.Ul: true, atom.Ol: true, atom.Li: true, atom.A: true, atom.Img: true, atom.Hr: true,
		atom.Pre: true, atom.Code: true,
		atom.Table: true, atom.Thead: true, atom.Tbody: true, atom.Tr: true, atom.Td: true, atom.Th: true,
	}
	// Elements removed together with their content.
	emailDroppedTags = map[atom.Atom]bool{
		atom.Script: true, atom.Style: true, atom.Iframe: true, atom.Object: true, atom.Embed: true,
		atom.Svg: true, atom.Math: true, atom.Head: true, atom.Title: true, atom.Noscript: true,
		atom.Template: true, atom.Form: true, atom.Input: true, atom.Textarea: true, atom.Select: true,
		atom.Button: true, atom.Frame: true, atom.Frameset: true, atom.Link: true, atom.Meta: true,
		atom.Base: true, atom.Audio: true, atom.Video: true, atom.Canvas: true,
	}
	emailTextAligns = []string{"left", "center", "right", "justify"}
)

type UserEmailDraft struct {
	Subject string                    `json:"subject"`
	HTML    string                    `json:"html"`
	Images  []common.InlineEmailImage `json:"images"`
}

// PreparedEmail is a draft that passed validation and sanitizing.
type PreparedEmail struct {
	Subject string
	HTML    string
	Images  []common.InlineEmailImage
}

// PrepareUserEmail validates the subject and images and reduces the body to
// the allowed formatting. Images the body does not reference are dropped.
func PrepareUserEmail(draft UserEmailDraft) (*PreparedEmail, error) {
	subject, err := normalizeEmailSubject(draft.Subject)
	if err != nil {
		return nil, err
	}
	if len(draft.HTML) > maxUserEmailHTMLBytes {
		return nil, ErrUserEmailBodyTooLong
	}
	if len(draft.Images) > maxUserEmailImages {
		return nil, ErrUserEmailImageCount
	}
	images := map[string]common.InlineEmailImage{}
	total := 0
	for _, image := range draft.Images {
		if !emailImageCID.MatchString(image.CID) || images[image.CID].CID != "" {
			return nil, ErrUserEmailImageInvalid
		}
		data, err := base64.StdEncoding.DecodeString(image.Content)
		if err != nil || len(data) == 0 {
			return nil, ErrUserEmailImageInvalid
		}
		total += len(data)
		if total > maxUserEmailImageBytes {
			return nil, ErrUserEmailImageSize
		}
		sniffed := http.DetectContentType(data)
		if !slices.Contains(emailImageTypes, image.ContentType) || sniffed != image.ContentType {
			return nil, ErrUserEmailImageInvalid
		}
		images[image.CID] = image
	}
	body, referenced, hasContent := sanitizeEmailHTML(draft.HTML, images)
	if !hasContent {
		return nil, ErrUserEmailBodyEmpty
	}
	prepared := &PreparedEmail{Subject: subject, HTML: body}
	for _, image := range draft.Images {
		if referenced[image.CID] {
			prepared.Images = append(prepared.Images, image)
		}
	}
	return prepared, nil
}

func normalizeEmailSubject(subject string) (string, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" || utf8.RuneCountInString(subject) > maxUserEmailSubjectRunes || strings.ContainsAny(subject, "\r\n\x00") {
		return "", ErrUserEmailSubject
	}
	return subject, nil
}

// sanitizeEmailHTML keeps allowed tags with safe attributes, unwraps unknown
// tags, drops active content, and reports which inline images the body uses
// and whether any text or image remains.
func sanitizeEmailHTML(input string, images map[string]common.InlineEmailImage) (string, map[string]bool, bool) {
	context := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := xhtml.ParseFragment(strings.NewReader(input), context)
	if err != nil {
		return "", nil, false
	}
	var out strings.Builder
	referenced := map[string]bool{}
	hasContent := false
	var walk func(node *xhtml.Node)
	walk = func(node *xhtml.Node) {
		switch node.Type {
		case xhtml.TextNode:
			if strings.TrimSpace(node.Data) != "" {
				hasContent = true
			}
			out.WriteString(html.EscapeString(node.Data))
			return
		case xhtml.ElementNode:
		default:
			return
		}
		if emailDroppedTags[node.DataAtom] {
			return
		}
		allowed := emailAllowedTags[node.DataAtom]
		if allowed {
			attributes, ok := emailAttributes(node, images, referenced)
			if !ok {
				return
			}
			if node.DataAtom == atom.Img {
				hasContent = true
			}
			out.WriteString("<" + node.Data + attributes + ">")
			if node.DataAtom == atom.Br || node.DataAtom == atom.Hr || node.DataAtom == atom.Img {
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if allowed {
			out.WriteString("</" + node.Data + ">")
		}
	}
	for _, node := range nodes {
		walk(node)
	}
	return out.String(), referenced, hasContent
}

// emailAttributes returns the kept attributes. ok is false for an image whose
// source is neither a web address nor an attached image.
func emailAttributes(node *xhtml.Node, images map[string]common.InlineEmailImage, referenced map[string]bool) (string, bool) {
	var out strings.Builder
	write := func(name, value string) {
		out.WriteString(" " + name + `="` + html.EscapeString(value) + `"`)
	}
	hasSource := false
	for _, attribute := range node.Attr {
		value := strings.TrimSpace(attribute.Val)
		switch {
		case attribute.Key == "href" && node.DataAtom == atom.A:
			if safeEmailURL(value, true) {
				write("href", value)
				write("rel", "noopener noreferrer")
			}
		case attribute.Key == "src" && node.DataAtom == atom.Img:
			if cid, ok := strings.CutPrefix(value, "cid:"); ok {
				if _, exists := images[cid]; exists {
					referenced[cid] = true
					write("src", value)
					hasSource = true
				}
			} else if safeEmailURL(value, false) {
				write("src", value)
				hasSource = true
			}
		case attribute.Key == "alt" || attribute.Key == "title":
			write(attribute.Key, value)
		case attribute.Key == "style":
			if align := styleTextAlign(value); align != "" {
				write("style", "text-align: "+align)
			}
		case attribute.Key == "align" && slices.Contains(emailTextAligns, strings.ToLower(value)):
			write("style", "text-align: "+strings.ToLower(value))
		}
	}
	if node.DataAtom == atom.Img && !hasSource {
		return "", false
	}
	return out.String(), true
}

func safeEmailURL(value string, allowMailto bool) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed.Host != "" && parsed.User == nil
	case "mailto":
		if !allowMailto {
			return false
		}
		_, err := mail.ParseAddress(parsed.Opaque)
		return err == nil
	}
	return false
}

func styleTextAlign(style string) string {
	for declaration := range strings.SplitSeq(style, ";") {
		name, value, ok := strings.Cut(declaration, ":")
		if !ok || strings.TrimSpace(strings.ToLower(name)) != "text-align" {
			continue
		}
		value = strings.TrimSpace(strings.ToLower(value))
		if slices.Contains(emailTextAligns, value) {
			return value
		}
	}
	return ""
}

// ValidEmailRecipient returns the user's address when it can receive mail.
func ValidEmailRecipient(user *model.User) (string, error) {
	address := strings.TrimSpace(user.Email)
	parsed, err := mail.ParseAddress(address)
	if address == "" || err != nil || parsed.Address != address || strings.ContainsAny(address, "\r\n\x00;") {
		return "", ErrUserEmailRecipient
	}
	return address, nil
}

// --- Account notice templates ---------------------------------------------

type EmailTemplate struct {
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

type AccountNoticeTemplates struct {
	Suspended EmailTemplate `json:"suspended"`
	Restored  EmailTemplate `json:"restored"`
}

// EmailTemplateVariables are the placeholders a notice template may use, as
// {{name}}; the server fills them in for each recipient.
var EmailTemplateVariables = []string{"display_name", "username", "email", "site_name", "site_url"}

func (templates AccountNoticeTemplates) For(kind string) (EmailTemplate, error) {
	switch kind {
	case model.UserEmailKindSuspended:
		return templates.Suspended, nil
	case model.UserEmailKindRestored:
		return templates.Restored, nil
	}
	return EmailTemplate{}, ErrUserEmailNoticeKind
}

// DefaultAccountNoticeTemplates are the built-in notices in lang.
func DefaultAccountNoticeTemplates(lang string) AccountNoticeTemplates {
	placeholders := map[string]any{
		"DisplayName": "{{display_name}}", "Username": "{{username}}",
		"SiteName": "{{site_name}}", "SiteURL": "{{site_url}}",
	}
	build := func(subjectKey, bodyKey string) EmailTemplate {
		return EmailTemplate{
			Subject: i18n.Translate(lang, subjectKey, placeholders),
			HTML:    i18n.Translate(lang, bodyKey, placeholders),
		}
	}
	return AccountNoticeTemplates{
		Suspended: build(i18n.MsgEmailAccountSuspendedSubject, i18n.MsgEmailAccountSuspendedBody),
		Restored:  build(i18n.MsgEmailAccountRestoredSubject, i18n.MsgEmailAccountRestoredBody),
	}
}

// SavedAccountNoticeTemplates returns root's templates, if any were saved.
func SavedAccountNoticeTemplates() (AccountNoticeTemplates, bool) {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[AccountNoticeTemplatesOption]
	common.OptionMapRWMutex.RUnlock()
	var templates AccountNoticeTemplates
	if strings.TrimSpace(raw) == "" || common.UnmarshalJsonStr(raw, &templates) != nil {
		return AccountNoticeTemplates{}, false
	}
	return templates, true
}

// EffectiveAccountNoticeTemplates are the saved templates, or the built-in
// ones in lang when root has not saved any.
func EffectiveAccountNoticeTemplates(lang string) AccountNoticeTemplates {
	if saved, ok := SavedAccountNoticeTemplates(); ok {
		return saved
	}
	return DefaultAccountNoticeTemplates(lang)
}

// SaveAccountNoticeTemplates validates and stores root's templates; bodies are
// stored sanitized, without inline images.
func SaveAccountNoticeTemplates(templates AccountNoticeTemplates) (AccountNoticeTemplates, error) {
	clean := func(template EmailTemplate) (EmailTemplate, error) {
		subject, err := normalizeEmailSubject(template.Subject)
		if err != nil {
			return EmailTemplate{}, err
		}
		if len(template.HTML) > maxUserEmailHTMLBytes {
			return EmailTemplate{}, ErrUserEmailBodyTooLong
		}
		body, _, hasContent := sanitizeEmailHTML(template.HTML, nil)
		if !hasContent {
			return EmailTemplate{}, ErrUserEmailTemplateEmpty
		}
		return EmailTemplate{Subject: subject, HTML: body}, nil
	}
	var saved AccountNoticeTemplates
	var err error
	if saved.Suspended, err = clean(templates.Suspended); err != nil {
		return saved, err
	}
	if saved.Restored, err = clean(templates.Restored); err != nil {
		return saved, err
	}
	encoded, err := common.Marshal(saved)
	if err != nil {
		return saved, err
	}
	return saved, model.UpdateOption(AccountNoticeTemplatesOption, string(encoded))
}

// EmailSiteValues are the site placeholders shared by every recipient.
func EmailSiteValues() map[string]string {
	return map[string]string{
		"site_name": common.SystemName,
		"site_url":  strings.TrimRight(system_setting.ServerAddress, "/"),
	}
}

// RenderAccountNotice fills a template for one user. Values are escaped in the
// body; the result is sanitized again before sending.
func RenderAccountNotice(template EmailTemplate, user *model.User) (*PreparedEmail, error) {
	values := EmailSiteValues()
	values["display_name"] = user.DisplayName
	if values["display_name"] == "" {
		values["display_name"] = user.Username
	}
	values["username"] = user.Username
	values["email"] = user.Email
	fill := func(text string, escape bool) string {
		return templateVar.ReplaceAllStringFunc(text, func(match string) string {
			name := templateVar.FindStringSubmatch(match)[1]
			value, ok := values[name]
			if !ok {
				return match
			}
			if escape {
				return html.EscapeString(value)
			}
			return value
		})
	}
	return PrepareUserEmail(UserEmailDraft{Subject: fill(template.Subject, false), HTML: fill(template.HTML, true)})
}
