package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"golang.org/x/image/draw"
	"image"
	"image/jpeg"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	// Importing as a side effect allows for the image library to check for these formats
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const (
	PlaceholderMessage = "No Content."
	MaxImageDimension  = 8192

	// MaxMailSize is the largest possible size mail can be, as per KD.
	MaxMailSize = 1578040
)

type Message struct {
	Attachment []byte
	Text       string
}

func readMultipartMessage(message io.Reader, boundary string) (*Message, error) {
	multipartReader := multipart.NewReader(message, boundary)

	var msg Message
	for {
		p, err := multipartReader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}

		contentType := p.Header.Get("Content-Type")
		contentTransferEncoding := p.Header.Get("Content-Transfer-Encoding")

		body, err := io.ReadAll(p)
		if err != nil {
			log.Fatal(err)
		}

		// Decode base64 if needed
		if strings.ToLower(contentTransferEncoding) == "base64" {
			body, err = base64.StdEncoding.DecodeString(string(body))
			if err != nil {
				log.Fatalf("Error decoding base64: %v", err)
			}
		}

		if strings.HasPrefix(contentType, "image/") {
			msg.Attachment = body
		} else if strings.HasPrefix(contentType, "text/") {
			msg.Text = removeNonUTF8Characters(string(body))
		} else {
			// We can't handle this, discard.
			continue
		}
	}

	return &msg, nil
}

func processInbound() {
	firstRun := true
	for {
		// Process immediately on boot.
		if !firstRun {
			// We want to do all our AWS operations every 30 minutes.
			time.Sleep(30 * time.Minute)
		}

		firstRun = false

		// Get all mail in the bucket.
		objects, err := GetObjects()
		if err != nil {
			ReportError(err)
			continue
		}

		for _, object := range objects {
			// Download the mail.
			objectData, err := DownloadObject(object)
			if err != nil {
				ReportError(err)
				continue
			}

			// Save to our server in a format the Wii can understand.
			err = readMessage(objectData)
			if err != nil {
				ReportError(err)
				continue
			}

			// Finally delete.
			err = DeleteObject(object)
			if err != nil {
				ReportError(err)
			}
		}
	}
}

func readMessage(email *s3.GetObjectOutput) error {
	// Parse the mail message
	defer email.Body.Close()
	msg, err := mail.ReadMessage(email.Body)
	if err != nil {
		log.Fatal(err)
	}

	fromRaw := msg.Header.Get("From")
	from, err := mail.ParseAddress(fromRaw)
	if err != nil {
		return err
	}

	toRaw := msg.Header.Get("To")
	to, err := mail.ParseAddress(toRaw)
	if err != nil {
		return err
	}

	subject := msg.Header.Get("Subject")

	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		return err
	}

	parts := &Message{}
	if strings.HasPrefix(mediaType, "multipart/") {
		parts, err = readMultipartMessage(msg.Body, params["boundary"])
		if err != nil {
			log.Fatal(err)
		}
	} else if strings.HasPrefix(mediaType, "text/") {
		// Body should be the message.
		msgBytes, err := io.ReadAll(msg.Body)
		if err != nil {
			return err
		}

		parts.Text = removeNonUTF8Characters(string(msgBytes))
	}

	if parts.Text == "" {
		parts.Text = PlaceholderMessage
	}

	formulatedMail, err := formulateMessage(from.Address, to.Address, subject, parts)
	if err != nil {
		return err
	}

	// We can do pretty much the exact same thing as the Wii send endpoint
	parsedWiiNumber := strings.Split(to.Address, "@")[0]
	_, err = pool.Exec(ctx, InsertMail, flakeNode.Generate(), formulatedMail, from.Address, parsedWiiNumber[1:])
	if err != nil {
		return err
	}

	return nil
}

func formulateMessage(from, to, subject string, msg *Message) (string, error) {
	boundary := generateBoundary()

	header := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n--%s\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Description: wiimail\r\n\r\n",
		from, to, subject, boundary, boundary)

	content := fmt.Sprint(header, msg.Text, strings.Repeat("\r\n", 3), "--", boundary, "--")

	// If there is no attachment, we are done here.
	if msg.Attachment == nil {
		return content, nil
	}

	decodedImage, _, err := image.Decode(bytes.NewReader(msg.Attachment))
	if err != nil {
		return content, nil
	}

	// Resize if needed
	decodedImage = resize(decodedImage)

	var jpegEncoded bytes.Buffer
	err = jpeg.Encode(bufio.NewWriter(&jpegEncoded), decodedImage, nil)
	if err != nil {
		return "", err
	}

	if jpegEncoded.Len()+len(content) > MaxMailSize {
		// If the image and content is too big, we can send just the content.
		return content, nil
	}

	base64Image := base64.StdEncoding.EncodeToString(jpegEncoded.Bytes())

	// According to RFC, 79 is the maximum allowed characters in a line.
	// Observations from messages sent by a Wii show 64 characters in a line before a line break.
	var fixedEncoding string
	for {
		if len(base64Image) >= 64 {
			fixedEncoding += base64Image[:64] + "\r\n"
			base64Image = base64Image[64:]
		} else {
			// To the end.
			fixedEncoding += base64Image[:]
			break
		}
	}

	return fmt.Sprint(header,
		msg.Text,
		strings.Repeat("\r\n", 3),
		"--", boundary, "\r\n",
		// Now we can put our image data.
		"Content-Type: image/jpeg; name=image.jpeg", "\r\n",
		"Content-Transfer-Encoding: base64", "\r\n",
		"Content-Disposition: attachment; filename=image.jpeg", "\r\n",
		"\r\n",
		fixedEncoding, "\r\n",
		"\r\n",
		"--", boundary, "--",
	), nil
}

// resize well resizes the image to what we want.
// Taken from https://stackoverflow.com/questions/22940724/go-resizing-images
func resize(originalImage image.Image) image.Image {
	width := originalImage.Bounds().Size().X
	height := originalImage.Bounds().Size().Y

	if width > MaxImageDimension {
		// Allows for proper scaling.
		height = height * MaxImageDimension / width
		width = MaxImageDimension
	}

	if height > MaxImageDimension {
		width = width * MaxImageDimension / height
		height = MaxImageDimension
	}

	if width != MaxImageDimension && height != MaxImageDimension {
		// No resize needs to occur.
		return originalImage
	}

	newImage := image.NewRGBA(image.Rect(0, 0, width, height))
	// BiLinear mode is the slowest out of the available but offers highest quality.
	draw.BiLinear.Scale(newImage, newImage.Bounds(), originalImage, originalImage.Bounds(), draw.Over, nil)
	return newImage
}

func removeNonUTF8Characters(message string) string {
	var buffer []byte

	for _, r := range message {
		if utf8.ValidRune(r) {
			buffer = append(buffer, []byte(string(r))...)
		}
	}

	return string(buffer)
}
