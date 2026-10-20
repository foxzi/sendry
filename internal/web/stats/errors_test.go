package stats

import "testing"

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name string
		text string
		want ErrorType
	}{
		{"empty", "", ""},
		{"blank", "  ", ""},

		// Address does not exist
		{"gmail no such account", "RCPT TO a@gmail.com failed: 550-5.1.1 The email account that you tried to reach does not exist.", ErrorAddressNotFound},
		{"postfix user unknown", "RCPT TO a@b.ru failed: 550 5.1.1 <a@b.ru>: Recipient address rejected: User unknown in virtual mailbox table", ErrorAddressNotFound},
		{"mail.ru no code", "RCPT TO a@mail.ru failed: 550 Message was not accepted -- invalid mailbox. Local mailbox a@mail.ru is unavailable: user not found", ErrorAddressNotFound},
		{"bad mailbox syntax", "RCPT TO x failed: 550 5.1.3 Bad recipient address syntax", ErrorAddressNotFound},
		{"mailbox disabled", "RCPT TO x failed: 550 5.2.1 Mailbox disabled, not accepting messages", ErrorAddressNotFound},
		{"551 user not local", "RCPT TO x failed: 551 User not local", ErrorAddressNotFound},

		// Mailbox full
		{"gmail over quota", "DATA close failed: 452-4.2.2 The email account that you tried to reach is over quota.", ErrorMailboxFull},
		{"yandex size limit", "RCPT TO x failed: 552 5.2.2 Mailbox size limit exceeded", ErrorMailboxFull},
		{"552 no enhanced", "DATA failed: 552 Requested mail action aborted: exceeded storage allocation", ErrorMailboxFull},
		{"quota keyword", "RCPT TO x failed: 550 user quota exceeded", ErrorMailboxFull},

		// Temporarily unavailable
		{"greylisting", "RCPT TO x failed: 450 4.2.0 <x>: Recipient address rejected: Greylisted, please try again later", ErrorTempUnavailable},
		{"451 local error", "RCPT TO x failed: 451 Requested action aborted: local error in processing", ErrorTempUnavailable},
		{"421 service unavailable", "HELO failed: 421 Service not available, closing transmission channel", ErrorTempUnavailable},
		{"4.7.0 try later", "MAIL FROM failed: 451 4.7.0 Temporary server error. Please try again later", ErrorTempUnavailable},

		// Domain unreachable
		{"mx lookup", "MX lookup failed for nodomain.example: lookup nodomain.example: no such host", ErrorDomainUnreachable},
		{"no mx hosts", "no MX hosts available for example.org", ErrorDomainUnreachable},
		{"connection refused", "connection failed to mx.example.org:25: dial tcp 192.0.2.1:25: connect: connection refused", ErrorDomainUnreachable},
		{"dial timeout", "connection failed to mx.example.org:25: dial tcp 192.0.2.1:25: i/o timeout", ErrorDomainUnreachable},
		{"5.1.2 bad domain", "RCPT TO x failed: 550 5.1.2 Host unknown", ErrorDomainUnreachable},
		{"5.4.4 no route", "RCPT TO x failed: 554 5.4.4 Unable to route", ErrorDomainUnreachable},

		// Rejected by server
		{"bad sender syntax", "MAIL FROM failed: 555 5.5.2 Syntax error", ErrorRejected},
		{"invalid mail from", "MAIL FROM failed: 501 Invalid MAIL FROM address", ErrorRejected},
		{"dmarc policy", "DATA close failed: 550-5.7.26 Unauthenticated email from example.com is not accepted due to domain's DMARC policy.", ErrorRejected},
		{"yandex policy", "RCPT TO x failed: 550 5.7.1 Policy rejection on the target address", ErrorRejected},
		{"554 transaction failed", "DATA close failed: 554 Transaction failed", ErrorRejected},
		{"message too big", "DATA close failed: 552 5.3.4 Message size exceeds fixed limit", ErrorRejected},

		// Spam
		{"spam content", "DATA close failed: 554 5.7.1 Message rejected as spam by Content Filtering", ErrorSpam},
		{"gmail unsolicited", "DATA close failed: 550-5.7.1 Our system has detected that this message is likely unsolicited mail.", ErrorSpam},
		{"mail.ru spam", "DATA close failed: 550 spam message rejected", ErrorSpam},

		// Blocked
		{"spamhaus", "RCPT TO x failed: 554 5.7.1 Service unavailable; Client host [192.0.2.1] blocked using zen.spamhaus.org", ErrorBlocked},
		{"outlook block list", "RCPT TO x failed: 550 5.7.1 Unfortunately, messages from [192.0.2.1] weren't sent. Part of their network is on our block list (S3150).", ErrorBlocked},
		{"reputation", "MAIL FROM failed: 550 5.7.1 Sender IP has poor reputation", ErrorBlocked},
		{"blocked", "MAIL FROM failed: 554 Your access to this mail system has been blocked", ErrorBlocked},

		// Temporary technical error
		{"expired certificate", "MAIL FROM failed: tls: failed to verify certificate: x509: certificate has expired or is not yet valid", ErrorTechnical},
		{"own rate limit", "recipient domain rate limit exceeded: gmail.com", ErrorTechnical},
		{"write timeout", "failed to write message data: write tcp 192.0.2.1:25: i/o timeout", ErrorTechnical},
		{"eof", "DATA close failed: EOF", ErrorTechnical},
		{"panel api down", "do request: Post \"http://localhost:8080/api/v1/send\": dial tcp [::1]:8080: connect: connection refused", ErrorTechnical},
		{"panel http status", "HTTP 500", ErrorTechnical},
		{"template not found", "template not found", ErrorTechnical},
		{"other 4xx", "DATA failed: 471 Local error", ErrorTechnical},

		// Unknown
		{"unrecognized", "something odd happened", ErrorUnknown},
		{"ip is not a code", "unexpected reply from 5.45.200.1", ErrorUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyError(tt.text); got != tt.want {
				t.Errorf("ClassifyError(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestErrorTypesHaveLabels(t *testing.T) {
	for _, et := range ErrorTypes {
		if et.Label() == string(et) {
			t.Errorf("error type %q has no label", et)
		}
		if ParseErrorType(string(et)) != et {
			t.Errorf("ParseErrorType(%q) did not round-trip", et)
		}
	}
	if ParseErrorType("bogus") != "" {
		t.Error("ParseErrorType accepted unknown value")
	}
}
