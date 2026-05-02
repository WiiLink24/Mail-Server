package main

import (
	"mime"
	"net/mail"
	"slices"
	"strings"
	"testing"
)

func TestEmailDecode(_t *testing.T) {
	message := `Content-Type: multipart/alternative;
 boundary="------------uPU7sYJfD0qHhIHRNc1JfkQy"
Message-ID: <025c664c-3c5d-4c27-bb41-1c89e77e21fd@btinternet.com>
Date: Sat, 2 May 2026 21:47:54 +0100
MIME-Version: 1.0
User-Agent: Mozilla Thunderbird
Content-Language: en-GB
To: contact@harrywalker.uk
From: Harry Walker <anonymised@btinternet.com>
Subject: Test email

This is a multi-part message in MIME format.
--------------uPU7sYJfD0qHhIHRNc1JfkQy
Content-Type: text/plain; charset=UTF-8; format=flowed
Content-Transfer-Encoding: 7bit

This is a test email to see how HTML formatting works


  wow a header



*some bold* /and italics/

--------------uPU7sYJfD0qHhIHRNc1JfkQy
Content-Type: text/html; charset=UTF-8
Content-Transfer-Encoding: 8bit

<!DOCTYPE html>
<html>
  <head>

    <meta http-equiv="content-type" content="text/html; charset=UTF-8">
  </head>
  <body>
    <p>This is a test email to see how HTML formatting works</p>
    <p><br>
    </p>
    <h1>wow a header</h1>
    <p><br>
    </p>
    <p><br>
    </p>
    <p><b>some bold</b>Â <i>and italics</i></p>
  </body>
</html>

--------------uPU7sYJfD0qHhIHRNc1JfkQy--
`
	expectedTo := "contact@harrywalker.uk"
	expectedFrom := "anonymised@btinternet.com"
	expectedSubject := "Test email"
	expectedText := `This is a test email to see how HTML formatting works


  wow a header



*some bold* /and italics/
`

	r := strings.NewReader(message)

	msg, err := mail.ReadMessage(r)
	if err != nil {
		_t.Fatal(err)
	}

	fromRaw := msg.Header.Get("From")
	from, err := mail.ParseAddress(fromRaw)
	if err != nil {
		_t.Fatal(err)
	}
	if from.Address != expectedFrom {
		_t.Errorf("Incorrect from address.\n\n Expected: '%s'\n\n Got: '%s'", expectedFrom, from.Address)
	}

	toRaw := msg.Header.Get("To")
	toList, err := mail.ParseAddressList(toRaw)
	if err != nil {
		_t.Fatal(err)
	}
	if !slices.ContainsFunc(toList, func(toAddress *mail.Address) bool {
		return toAddress.Address == expectedTo
	}) {
		_t.Errorf("Incorrect to address.\n\n Expected: '%s'\n\n Got: '%s'", expectedTo, toList)
	}

	subject := msg.Header.Get("Subject")
	if subject != expectedSubject {
		_t.Errorf("Incorrect subject.\n\n Expected: '%s'\n\n Got: '%s'", expectedSubject, subject)
	}

	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		_t.Fatal(err)
	}

	parts := &Message{}
	if strings.HasPrefix(mediaType, "multipart/") {
		parts, err = readMultipartMessage(msg.Body, params["boundary"])
		if err != nil {
			_t.Fatal(err)
		}
	}

	if parts.Text != expectedText {
		_t.Errorf("Incorrect body.\n\n Expected: '%s'\n\n Got: '%s'", expectedText, parts.Text)
	}
}
