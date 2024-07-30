package main

import (
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

const DeleteSentMail = `DELETE FROM mail WHERE is_sent = true AND recipient = $1`

func _delete(c *gin.Context) {
	mlid := c.PostForm("mlid")
	password := c.PostForm("passwd")

	err := validatePassword(c.Copy(), mlid, password)
	if errors.Is(err, ErrInvalidCredentials) {
		cgi := GenCGIError(250, err.Error())
		ReportError(err)
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	} else if err != nil {
		cgi := GenCGIError(551, "An error has occurred while querying the database.")
		ReportError(err)
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	}

	// We are sent the number of messages to delete, however we will ignore it as
	// we set a flag for the messages that were already sent.
	delNum := c.PostForm("delnum")
	// Integer checking
	intDelNum, err := strconv.ParseInt(delNum, 10, 64)
	if err != nil {
		cgi := GenCGIError(340, "Invalid delnum value was passed")
		ReportError(err)
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	}

	_, err = pool.Exec(c.Copy(), DeleteSentMail, mlid[1:])
	if err != nil {
		cgi := GenCGIError(541, "An error has occurred while deleting the messages from the database.")
		ReportError(err)
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	}

	cgi := CGIResponse{
		code:    100,
		message: "Success.",
		other: []KV{
			{
				key:   "deletenum",
				value: delNum,
			},
		},
	}

	if config.UseDatadog {
		err = dataDog.Incr("mail.deleted_mail", nil, float64(intDelNum))
		if err != nil {
			ReportError(err)
		}
	}

	c.String(http.StatusOK, ConvertToCGI(cgi))
}
