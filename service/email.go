package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"html"
	"log"
	"math/big"
	"mime"
	"net"
	netmail "net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

const (
	emailCodeValidity       = 10 * time.Minute
	emailCodeResendInterval = 60 * time.Second
	emailIPWindow           = time.Minute
	emailIPLimit            = 5
	emailMaxAttempts        = 5
)

var emailSendMu sync.Mutex

func SendRegistrationEmailCode(rawEmail string, requestIP string) error {
	emailSendMu.Lock()
	defer emailSendMu.Unlock()

	settings, err := repository.GetSettings()
	if err != nil {
		return err
	}
	settings = normalizeSettings(settings)
	if settings.Public.Auth.AllowRegister != nil && !*settings.Public.Auth.AllowRegister {
		return safeMessageError{message: "当前未开放注册"}
	}
	email, err := normalizeEmailAddress(rawEmail)
	if err != nil {
		return safeMessageError{message: "请输入有效的电子邮箱"}
	}
	if err := validateRegistrationEmail(email, settings.Public.Auth); err != nil {
		return err
	}
	if _, exists, err := repository.GetUserByEmail(email); err != nil {
		return err
	} else if exists {
		return safeMessageError{message: "该邮箱已注册"}
	}

	verificationID := emailVerificationID(email)
	currentTime := time.Now()
	_ = repository.DeleteExpiredEmailVerifications(currentTime.Format(time.RFC3339))
	current, exists, err := repository.GetEmailVerification(verificationID)
	if err != nil {
		return err
	}
	if exists {
		if sentAt, parseErr := time.Parse(time.RFC3339, current.SentAt); parseErr == nil && currentTime.Sub(sentAt) < emailCodeResendInterval {
			return safeMessageError{message: "验证码发送过于频繁，请稍后再试"}
		}
	}
	requestIP = strings.TrimSpace(requestIP)
	if requestIP != "" {
		total, countErr := repository.CountRecentEmailVerificationsByIP(requestIP, currentTime.Add(-emailIPWindow).Format(time.RFC3339))
		if countErr != nil {
			return countErr
		}
		if total >= emailIPLimit {
			return safeMessageError{message: "验证码请求过于频繁，请稍后再试"}
		}
	}

	code, err := generateEmailCode()
	if err != nil {
		return err
	}
	codeHash := emailCodeHash(email, code)
	timestamp := currentTime.Format(time.RFC3339)
	createdAt := timestamp
	if exists && current.CreatedAt != "" {
		createdAt = current.CreatedAt
	}
	verification := model.EmailVerification{
		ID:        verificationID,
		Email:     email,
		CodeHash:  codeHash,
		RequestIP: requestIP,
		ExpiresAt: currentTime.Add(emailCodeValidity).Format(time.RFC3339),
		SentAt:    timestamp,
		Attempts:  0,
		UsedAt:    "",
		CreatedAt: createdAt,
		UpdatedAt: timestamp,
	}
	if err := repository.SaveEmailVerification(verification); err != nil {
		return err
	}
	if err := sendRegistrationCodeEmail(settings.Private.Email, email, code); err != nil {
		_ = repository.DeleteEmailVerification(verificationID, codeHash)
		log.Printf("send registration verification email failed: %v", err)
		return safeMessageError{message: "验证码发送失败，请联系管理员检查邮件配置"}
	}
	return nil
}

func normalizeEmailAddress(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || len(value) > 254 || strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("invalid email")
	}
	address, err := netmail.ParseAddress(value)
	if err != nil || strings.ToLower(address.Address) != value {
		return "", fmt.Errorf("invalid email")
	}
	parts := strings.Split(value, "@")
	if len(parts) != 2 || parts[0] == "" || normalizeEmailDomain(parts[1]) != parts[1] {
		return "", fmt.Errorf("invalid email")
	}
	return value, nil
}

func validateRegistrationEmail(email string, setting model.PublicAuthSetting) error {
	if !setting.EmailDomainRestriction {
		return nil
	}
	domain := email[strings.LastIndex(email, "@")+1:]
	for _, allowed := range setting.EmailDomains {
		if domain == strings.ToLower(strings.TrimSpace(strings.TrimPrefix(allowed, "@"))) {
			return nil
		}
	}
	return safeMessageError{message: "该邮箱类型暂不支持注册"}
}

func generateEmailCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", value.Int64()+100000), nil
}

func emailVerificationID(email string) string {
	sum := sha256.Sum256([]byte(email))
	return hex.EncodeToString(sum[:])
}

func emailCodeHash(email string, code string) string {
	mac := hmac.New(sha256.New, []byte(config.Cfg.JWTSecret))
	_, _ = mac.Write([]byte(email))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

func verifyEmailCodeHash(expected string, actual string) bool {
	return subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) == 1
}

func sendRegistrationCodeEmail(setting model.EmailSetting, receiver string, code string) error {
	subject := "道生画境注册验证码"
	body := fmt.Sprintf("<div style=\"font-family:Arial,sans-serif;line-height:1.7;color:#1f2937\"><h2 style=\"margin:0 0 16px\">注册验证码</h2><p>你的验证码是：</p><p style=\"font-size:30px;font-weight:700;letter-spacing:8px;margin:18px 0\">%s</p><p>验证码 10 分钟内有效，请勿转发给他人。</p></div>", code)
	return sendHTMLEmail(setting, receiver, subject, body)
}

func SendOrganizationInvitationEmail(receiver string, organizationName string, role model.OrganizationRole) error {
	settings, err := repository.GetSettings()
	if err != nil {
		return err
	}
	roleName := map[model.OrganizationRole]string{model.OrganizationRoleAdmin: "管理员", model.OrganizationRoleMember: "成员", model.OrganizationRoleReviewer: "审核人"}[role]
	organizationName = html.EscapeString(organizationName)
	action := ""
	if baseURL := strings.TrimRight(strings.TrimSpace(config.Cfg.PublicBaseURL), "/"); baseURL != "" {
		action = fmt.Sprintf("<p><a href=\"%s/commerce\" style=\"display:inline-block;padding:10px 18px;background:#111827;color:#fff;text-decoration:none\">打开企业中心</a></p>", html.EscapeString(baseURL))
	}
	body := fmt.Sprintf("<div style=\"font-family:Arial,sans-serif;line-height:1.7;color:#1f2937\"><h2 style=\"margin:0 0 16px\">企业协作邀请</h2><p>你已被邀请加入企业 <strong>%s</strong>，角色为 <strong>%s</strong>。</p><p>请使用当前邮箱登录后，在企业中心接受邀请。邀请 7 天内有效。</p>%s</div>", organizationName, roleName, action)
	return sendHTMLEmail(settings.Private.Email, receiver, "道生画境企业协作邀请", body)
}

func sendHTMLEmail(setting model.EmailSetting, receiver string, subject string, body string) error {
	setting = normalizeEmailSetting(setting)
	if setting.SMTPHost == "" || setting.SMTPPort <= 0 || setting.SMTPFromEmail == "" {
		return fmt.Errorf("SMTP is not configured")
	}
	if setting.SMTPSecurity == "none" && (setting.SMTPUsername != "" || setting.SMTPPassword != "") {
		return fmt.Errorf("SMTP credentials require TLS")
	}
	if (setting.SMTPUsername == "") != (setting.SMTPPassword == "") {
		return fmt.Errorf("SMTP username and password must be configured together")
	}
	if _, err := normalizeEmailAddress(setting.SMTPFromEmail); err != nil {
		return fmt.Errorf("invalid SMTP sender address")
	}

	fromName := setting.SMTPFromName
	if fromName == "" {
		fromName = "道生画境"
	}
	if strings.ContainsAny(fromName, "\r\n") {
		return fmt.Errorf("invalid SMTP sender name")
	}
	from := (&netmail.Address{Name: fromName, Address: setting.SMTPFromEmail}).String()
	message := strings.Join([]string{
		"From: " + from,
		"To: " + receiver,
		"Subject: " + mime.QEncoding.Encode("UTF-8", subject),
		"Date: " + time.Now().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		body,
	}, "\r\n")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	address := net.JoinHostPort(setting.SMTPHost, strconv.Itoa(setting.SMTPPort))
	tlsConfig := &tls.Config{ServerName: setting.SMTPHost, MinVersion: tls.VersionTLS12}
	var connection net.Conn
	var err error
	if setting.SMTPSecurity == "ssl" {
		connection, err = tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return err
	}
	_ = connection.SetDeadline(time.Now().Add(15 * time.Second))
	client, err := smtp.NewClient(connection, setting.SMTPHost)
	if err != nil {
		_ = connection.Close()
		return err
	}
	defer client.Close()
	if setting.SMTPSecurity == "starttls" {
		if supported, _ := client.Extension("STARTTLS"); !supported {
			return fmt.Errorf("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if setting.SMTPUsername != "" {
		if err := client.Auth(smtp.PlainAuth("", setting.SMTPUsername, setting.SMTPPassword, setting.SMTPHost)); err != nil {
			return err
		}
	}
	if err := client.Mail(setting.SMTPFromEmail); err != nil {
		return err
	}
	if err := client.Rcpt(receiver); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
