package main

import (
	"encoding/xml"
	"net/http"
)

// Response describes the inner response format, along with common fields across requests.
type Response struct {
	request  *http.Request
	writer   *http.ResponseWriter
	response string
	cgi      CGIResponse
}

type CGIResponse struct {
	code    int
	message string
	other   []KV
}

// KV represents a key-value field.
type KV struct {
	key   string
	value string
}

type Config struct {
	XMLName     xml.Name `xml:"Config"`
	Address     string   `xml:"Address"`
	SQLAddress  string   `xml:"SQLAddress"`
	SQLUser     string   `xml:"SQLUser"`
	SQLPass     string   `xml:"SQLPass"`
	SQLDB       string   `xml:"SQLDB"`
	SentryDSN   string   `xml:"SentryDSN"`
	SendGridKey string   `xml:"SendGridKey"`
	UseDatadog  bool     `xml:"UseDatadog"`
}
