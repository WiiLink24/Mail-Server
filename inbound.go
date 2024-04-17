package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/image/draw"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"unicode/utf8"

	// Importing as a side effect allows for the image library to check for these formats
	_ "image/gif"
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

func inbound(r *Response) string {
	err := r.request.ParseMultipartForm(-1)
	if err != nil {
		r.cgi = GenCGIError(350, "Failed to parse mail.")
		ReportError(err)
		return ConvertToCGI(r.cgi)
	}

	if r.request.Form.Get("from") == "" || r.request.Form.Get("to") == "" {
		(*r.writer).WriteHeader(http.StatusBadRequest)
		return ""
	}

	message := r.request.Form.Get("text")
	if message == "" {
		message = PlaceholderMessage
	}

	// Sanitize the message
	message = removeNonUTF8Characters(message)

	fromRaw := r.request.Form.Get("from")
	from, err := mail.ParseAddress(fromRaw)
	if err != nil {
		(*r.writer).WriteHeader(http.StatusBadRequest)
		ReportError(err)
		return ""
	}

	toRaw := r.request.Form.Get("to")
	to, err := mail.ParseAddress(toRaw)
	if err != nil {
		(*r.writer).WriteHeader(http.StatusBadRequest)
		ReportError(err)
		return ""
	}

	subject := r.request.Form.Get("subject")

	type File struct {
		Filename string `go:"filename"`
		Charset  string `go:"charset"`
		Type     string `go:"type"`
	}

	var attachment []byte
	attachmentInfo := make(map[string]File)
	err = json.Unmarshal([]byte(r.request.Form.Get("attachment-info")), &attachmentInfo)
	if err == nil {
		hasImage := false
		hasAttachedText := false

		for name, _attachment := range attachmentInfo {
			attachmentData, _, err := r.request.FormFile(name)
			if errors.Is(err, http.ErrMissingFile) {
				// We don't care if there's nothing, it'll just stay nil.
			} else if err != nil {
				(*r.writer).WriteHeader(http.StatusBadRequest)
				ReportError(err)
				return ""
			} else {
				if strings.Contains(_attachment.Type, "image") && hasImage == false {
					attachment, err = io.ReadAll(attachmentData)
					if err != nil {
						(*r.writer).WriteHeader(http.StatusBadRequest)
						ReportError(err)
						return ""
					}
					hasImage = true
				} else if strings.Contains(_attachment.Type, "text") && hasAttachedText == false && message == PlaceholderMessage {
					attachedText, err := io.ReadAll(attachmentData)
					message = string(attachedText)
					if err != nil {
						(*r.writer).WriteHeader(http.StatusBadRequest)
						ReportError(err)
						return ""
					}

					hasAttachedText = true
				}
			}
		}
	}

	formulatedMail, err := formulateMessage(from.Address, to.Address, subject, message, attachment)
	if err != nil {
		(*r.writer).WriteHeader(http.StatusInternalServerError)
		ReportError(err)
		return ""
	}

	// We can do pretty much the exact same thing as the Wii send endpoint
	parsedWiiNumber := strings.Split(to.Address, "@")[0]
	_, err = pool.Exec(ctx, InsertMail, flakeNode.Generate(), formulatedMail, from.Address, parsedWiiNumber[1:])
	if err != nil {
		(*r.writer).WriteHeader(http.StatusInternalServerError)
		ReportError(err)
		return ""
	}

	(*r.writer).WriteHeader(http.StatusOK)
	return ""
}

func formulateMessage(from, to, subject, body string, attachment []byte) (string, error) {
	boundary := generateBoundary()

	header := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n--%s\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Description: wiimail\r\n\r\n",
		from, to, subject, boundary, boundary)

	content := fmt.Sprint(header, body, strings.Repeat("\r\n", 3), "--", boundary, "--")

	// If there is no attachment, we are done here.
	if attachment == nil {
		return content, nil
	}

	decodedImage, _, err := image.Decode(bytes.NewReader(attachment))
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
		body,
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
