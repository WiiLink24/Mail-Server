package main

import (
	"errors"
	"strconv"
)

const DeleteSentMail = `DELETE FROM mail WHERE is_sent = true AND recipient = $1`

func _delete(r *Response) string {
	err := r.request.ParseForm()
	if err != nil {
		r.cgi = GenCGIError(350, "Failed to parse POST form.")
		return ConvertToCGI(r.cgi)
	}

	mlid := r.request.Form.Get("mlid")
	password := r.request.Form.Get("passwd")

	err = validatePassword(mlid, password)
	if errors.Is(err, ErrInvalidCredentials) {
		r.cgi = GenCGIError(250, err.Error())
		ReportError(err)
		return ConvertToCGI(r.cgi)
	} else if err != nil {
		r.cgi = GenCGIError(551, "An error has occurred while querying the database.")
		ReportError(err)
		return ConvertToCGI(r.cgi)
	}

	// We are sent the number of messages to delete, however we will ignore it as
	// we set a flag for the messages that were already sent.
	delNum := r.request.Form.Get("delnum")
	// Integer checking
	intDelNum, err := strconv.ParseInt(delNum, 10, 64)
	if err != nil {
		r.cgi = GenCGIError(340, "Invalid delnum value was passed")
		ReportError(err)
		return ConvertToCGI(r.cgi)
	}

	_, err = pool.Exec(ctx, DeleteSentMail, mlid[1:])
	if err != nil {
		r.cgi = GenCGIError(541, "An error has occurred while deleting the messages from the database.")
		ReportError(err)
		return ConvertToCGI(r.cgi)
	}

	r.cgi = CGIResponse{
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

	return ConvertToCGI(r.cgi)
}
