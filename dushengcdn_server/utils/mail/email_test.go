package mail

import (
	"bufio"
	"crypto/tls"
	"dushengcdn/common"
	"net"
	"net/smtp"
	"strings"
	"testing"
)

func TestSMTPTLSConfigVerifiesCertificates(t *testing.T) {
	config := smtpTLSConfig("smtp.example.com")
	if config.InsecureSkipVerify {
		t.Fatal("SMTP TLS config must verify certificates")
	}
	if config.ServerName != "smtp.example.com" {
		t.Fatalf("unexpected server name: %s", config.ServerName)
	}
	if config.MinVersion < tls.VersionTLS12 {
		t.Fatalf("expected TLS 1.2 or newer, got %x", config.MinVersion)
	}
}

func TestParseSMTPRecipients(t *testing.T) {
	recipients, header, err := parseSMTPRecipients("Alice <alice@example.com>; bob@example.com")
	if err != nil {
		t.Fatalf("parse recipients: %v", err)
	}
	if strings.Join(recipients, ",") != "alice@example.com,bob@example.com" {
		t.Fatalf("unexpected recipients: %#v", recipients)
	}
	if !strings.Contains(header, "Alice") || !strings.Contains(header, "alice@example.com") || !strings.Contains(header, "bob@example.com") {
		t.Fatalf("unexpected to header: %s", header)
	}
}

func TestParseSMTPRecipientsRejectsHeaderInjection(t *testing.T) {
	if _, _, err := parseSMTPRecipients("victim@example.com\r\nBcc: attacker@example.com"); err == nil {
		t.Fatal("expected recipient header injection to be rejected")
	}
}

func TestBuildSMTPMessageSanitizesFromHeader(t *testing.T) {
	originalSystemName := common.SystemName
	originalAccount := common.SMTPAccount
	common.SystemName = "DuSheng\r\nBcc: attacker@example.com"
	common.SMTPAccount = "sender@example.com"
	t.Cleanup(func() {
		common.SystemName = originalSystemName
		common.SMTPAccount = originalAccount
	})

	_, _, message, err := buildSMTPMessage("hello", "receiver@example.com", "content")
	if err != nil {
		t.Fatalf("build message: %v", err)
	}
	if strings.Contains(string(message), "Bcc: attacker@example.com") {
		t.Fatalf("message contains injected header: %s", string(message))
	}
	if !strings.Contains(string(message), "From:") {
		t.Fatalf("message missing from header: %s", string(message))
	}
}

func TestSendSMTPMessageRejectsAuthWithoutSTARTTLS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen smtp fixture: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		reader := bufio.NewReader(conn)
		writer := bufio.NewWriter(conn)
		writeSMTPLine(t, writer, "220 smtp.example.com ESMTP")
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			command := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(command, "EHLO"):
				writeSMTPLine(t, writer, "250-smtp.example.com")
				writeSMTPLine(t, writer, "250 AUTH PLAIN")
			case strings.HasPrefix(command, "QUIT"):
				writeSMTPLine(t, writer, "221 bye")
				return
			default:
				writeSMTPLine(t, writer, "500 unsupported")
			}
		}
	}()

	auth := smtp.PlainAuth("", "sender@example.com", "secret", "smtp.example.com")
	err = sendSMTPMessage(listener.Addr().String(), "smtp.example.com", auth, "sender@example.com", []string{"receiver@example.com"}, []byte("hello"), false)
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("expected STARTTLS error, got %v", err)
	}
	<-done
}

func writeSMTPLine(t *testing.T, writer *bufio.Writer, line string) {
	t.Helper()
	if _, err := writer.WriteString(line + "\r\n"); err != nil {
		t.Errorf("write smtp fixture line: %v", err)
		return
	}
	if err := writer.Flush(); err != nil {
		t.Errorf("flush smtp fixture line: %v", err)
	}
}
