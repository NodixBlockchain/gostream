package main

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
)

type site struct {
	siteURL    string
	siteOrigin string
	userid     int
}

func (mysite *site) checkCRSF(token string) error {
	client := &http.Client{}

	url := mysite.siteURL + "/Membres/crossLogin/" + token

	req, httpErr := http.NewRequest("GET", url, nil)
	if httpErr != nil {
		return fmt.Errorf("http.NewRequest failed : %s %v\r\n", url, httpErr)
	}

	resp, httpErr := client.Do(req)
	if httpErr != nil {
		return fmt.Errorf("client.Do(req) failed : %s %v\r\n", url, httpErr)

	} else {

		body, httpErr := io.ReadAll(resp.Body)

		if httpErr != nil {
			return fmt.Errorf("io.ReadAll(resp.Body) failed : %s %v\r\n", url, httpErr)
		}

		mysite.userid, httpErr = strconv.Atoi(string(body))
		if httpErr != nil {
			return fmt.Errorf("read body failed : %s %v\r\n", url, httpErr)
		}
	}

	return nil
}
