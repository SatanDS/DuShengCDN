package mail

import (
	"crypto/tls"
	"dushengcdn/common"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	stdmail "net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

var smtpConnectTimeout = 10 * time.Second

func SendEmail(subject string, receiver string, content string) error {
	server := strings.TrimSpace(common.SMTPServer)
	if server == "" {
		return errors.New("SMTP 服务器不能为空")
	}
	if common.SMTPPort <= 0 {
		return errors.New("SMTP 端口无效")
	}

	from, recipients, message, err := buildSMTPMessage(subject, receiver, content)
	if err != nil {
		return err
	}
	auth := smtp.PlainAuth("", from, common.SMTPToken, server)
	addr := net.JoinHostPort(server, strconv.Itoa(common.SMTPPort))
	return sendSMTPMessage(addr, server, auth, from, recipients, message, common.SMTPPort == 465)
}

func buildSMTPMessage(subject string, receiver string, content string) (string, []string, []byte, error) {
	fromAddress, err := parseSMTPAddress(common.SMTPAccount)
	if err != nil {
		return "", nil, nil, fmt.Errorf("SMTP 账户无效: %w", err)
	}
	recipients, toHeader, err := parseSMTPRecipients(receiver)
	if err != nil {
		return "", nil, nil, err
	}
	encodedSubject := fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(subject)))
	fromHeader := (&stdmail.Address{
		Name:    sanitizeSMTPHeaderDisplayName(common.SystemName),
		Address: fromAddress.Address,
	}).String()
	message := []byte(fmt.Sprintf("To: %s\r\n"+
		"From: %s\r\n"+
		"Subject: %s\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n",
		toHeader, fromHeader, encodedSubject, content))
	return fromAddress.Address, recipients, message, nil
}

func parseSMTPAddress(raw string) (*stdmail.Address, error) {
	if strings.ContainsAny(raw, "\r\n") {
		return nil, errors.New("地址包含非法换行")
	}
	address, err := stdmail.ParseAddress(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(address.Address) == "" {
		return nil, errors.New("地址为空")
	}
	return address, nil
}

func parseSMTPRecipients(raw string) ([]string, string, error) {
	if strings.ContainsAny(raw, "\r\n") {
		return nil, "", errors.New("收件人包含非法换行")
	}
	normalized := strings.ReplaceAll(strings.TrimSpace(raw), ";", ",")
	addresses, err := stdmail.ParseAddressList(normalized)
	if err != nil {
		return nil, "", fmt.Errorf("收件人地址无效: %w", err)
	}
	if len(addresses) == 0 {
		return nil, "", errors.New("收件人不能为空")
	}
	recipients := make([]string, 0, len(addresses))
	headerAddresses := make([]string, 0, len(addresses))
	for _, address := range addresses {
		recipient := strings.TrimSpace(address.Address)
		if recipient == "" {
			continue
		}
		recipients = append(recipients, recipient)
		headerAddresses = append(headerAddresses, address.String())
	}
	if len(recipients) == 0 {
		return nil, "", errors.New("收件人不能为空")
	}
	return recipients, strings.Join(headerAddresses, ", "), nil
}

func sendSMTPMessage(addr string, server string, auth smtp.Auth, from string, recipients []string, message []byte, implicitTLS bool) error {
	client, err := newSMTPClient(addr, server, implicitTLS)
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Close()
	}()

	if !implicitTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err = client.StartTLS(smtpTLSConfig(server)); err != nil {
				return err
			}
		} else if auth != nil {
			return errors.New("SMTP 服务器不支持 STARTTLS，拒绝在明文连接上传输认证信息")
		}
	}
	if auth != nil {
		if ok, _ := client.Extension("AUTH"); !ok {
			return errors.New("SMTP 服务器不支持认证")
		}
		if err = client.Auth(auth); err != nil {
			return err
		}
	}
	if err = client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range recipients {
		if err = client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func newSMTPClient(addr string, server string, implicitTLS bool) (*smtp.Client, error) {
	dialer := &net.Dialer{Timeout: smtpConnectTimeout}
	if implicitTLS {
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, smtpTLSConfig(server))
		if err != nil {
			return nil, err
		}
		return smtp.NewClient(conn, server)
	}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return smtp.NewClient(conn, server)
}

func smtpTLSConfig(server string) *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: strings.TrimSpace(server),
	}
}

func sanitizeSMTPHeaderDisplayName(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	if line := strings.TrimSpace(strings.Split(value, "\n")[0]); line != "" {
		return line
	}
	return "DuShengCDN"
}
