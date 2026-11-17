package common

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"time"
)

// InlineEmailImage is an image embedded in an HTML email and referenced from
// the body as cid:<CID>. Content is standard base64; callers validate type
// and size before sending.
type InlineEmailImage struct {
	CID         string `json:"cid"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"`
}

// resendAttachment follows https://resend.com/docs/api-reference/emails/send-email:
// an attachment with content_id is shown inline where the body uses cid:.
type resendAttachment struct {
	Filename    string `json:"filename"`
	Content     string `json:"content"`
	ContentType string `json:"content_type"`
	ContentID   string `json:"content_id"`
}

var inlineImageExtensions = map[string]string{
	"image/png":  "png",
	"image/jpeg": "jpg",
	"image/gif":  "gif",
	"image/webp": "webp",
}

// SendEmailWithImages sends an HTML email with inline images through the
// configured provider. Without images it is the same as SendEmail.
func SendEmailWithImages(subject string, receiver string, content string, images []InlineEmailImage) error {
	if len(images) == 0 {
		return SendEmail(subject, receiver, content)
	}
	OptionMapRWMutex.RLock()
	provider := OptionMap["EmailProvider"]
	resendKey := OptionMap["ResendApiKey"]
	resendFrom := OptionMap["ResendFrom"]
	OptionMapRWMutex.RUnlock()

	switch provider {
	case "resend":
		attachments := make([]resendAttachment, 0, len(images))
		for _, image := range images {
			attachments = append(attachments, resendAttachment{
				Filename:    image.CID + "." + inlineImageExtensions[image.ContentType],
				Content:     image.Content,
				ContentType: image.ContentType,
				ContentID:   image.CID,
			})
		}
		return sendResendEmail(resendKey, resendFrom, subject, receiver, content, attachments...)
	case "", "smtp":
	default:
		return NewMessage("Email sending method must be SMTP or Resend")
	}

	if SMTPFrom == "" { // for compatibility
		SMTPFrom = SMTPAccount
	}
	if SMTPServer == "" && SMTPAccount == "" {
		return NewMessage("SMTP server is not configured")
	}
	id, err := generateMessageID()
	if err != nil {
		return err
	}
	body, boundary, err := buildRelatedMIMEBody(content, images)
	if err != nil {
		return err
	}
	encodedSubject := fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(subject)))
	mail := []byte(fmt.Sprintf("To: %s\r\n"+
		"From: %s <%s>\r\n"+
		"Subject: %s\r\n"+
		"Date: %s\r\n"+
		"Message-ID: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: multipart/related; boundary=%q\r\n\r\n%s",
		receiver, SystemName, SMTPFrom, encodedSubject, time.Now().Format(time.RFC1123Z), id, boundary, body))
	return deliverSMTPMail(receiver, mail)
}

// buildRelatedMIMEBody returns a multipart/related body: the quoted-printable
// HTML part followed by one base64 part per image, wrapped at 76 characters.
func buildRelatedMIMEBody(content string, images []InlineEmailImage) (string, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	htmlPart, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {"text/html; charset=UTF-8"},
		"Content-Transfer-Encoding": {"quoted-printable"},
	})
	if err != nil {
		return "", "", err
	}
	encoded := quotedprintable.NewWriter(htmlPart)
	if _, err := encoded.Write([]byte(content)); err != nil {
		return "", "", err
	}
	if err := encoded.Close(); err != nil {
		return "", "", err
	}
	for _, image := range images {
		part, err := writer.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {image.ContentType},
			"Content-ID":                {"<" + image.CID + ">"},
			"Content-Disposition":       {"inline; filename=\"" + image.CID + "." + inlineImageExtensions[image.ContentType] + "\""},
			"Content-Transfer-Encoding": {"base64"},
		})
		if err != nil {
			return "", "", err
		}
		for data := image.Content; len(data) > 0; {
			line := data[:min(76, len(data))]
			data = data[len(line):]
			if _, err := part.Write([]byte(line + "\r\n")); err != nil {
				return "", "", err
			}
		}
	}
	if err := writer.Close(); err != nil {
		return "", "", err
	}
	return body.String(), writer.Boundary(), nil
}
