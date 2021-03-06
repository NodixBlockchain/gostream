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
	enable     bool
}

func (mysite *site) checkCRSF(token string) (int, error) {
	client := &http.Client{}

	url := mysite.siteURL + "/Membres/crossLogin/" + token

	req, httpErr := http.NewRequest("GET", url, nil)
	if httpErr != nil {
		return 0, fmt.Errorf("http.NewRequest failed : %s %v\r\n", url, httpErr)
	}

	resp, httpErr := client.Do(req)
	if httpErr != nil {
		return 0, fmt.Errorf("client.Do(req) failed : %s %v\r\n", url, httpErr)

	}

	body, httpErr := io.ReadAll(resp.Body)

	if httpErr != nil {
		return 0, fmt.Errorf("io.ReadAll(resp.Body) failed : %s %v\r\n", url, httpErr)
	}

	userid, httpErr := strconv.Atoi(string(body))
	if httpErr != nil {
		return 0, fmt.Errorf("read body failed : %s %v\r\n", url, httpErr)
	}
	if userid <= 0 {
		return 0, fmt.Errorf("userid null : %s %d\r\n", url, userid)
	}

	return userid, nil
}

func (mysite *site) checkAppel(Destination int, token string) error {
	client := &http.Client{}

	url := mysite.siteURL + "/Membres/peuxAppeller/" + strconv.Itoa(Destination) + "/" + token

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

		ok, httpErr := strconv.Atoi(string(body))
		if ok != 1 {
			return fmt.Errorf("body error : '%s' on URL '%s' \r\n", string(body), url)
		}
	}

	return nil
}

func (mysite *site) newInput(roomID int, token string, on int) error {
	client := &http.Client{}

	url := mysite.siteURL + "/Groupes/envoieAudioGroup/" + strconv.Itoa(roomID) + "/" + token + "/" + strconv.Itoa(on)

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

		ok, httpErr := strconv.Atoi(string(body))
		if ok != 1 {
			return fmt.Errorf("body error : '%s' on URL '%s' \r\n", string(body), url)
		}
	}

	return nil
}

func (mysite *site) newListener(roomID int, token string, on int) error {
	client := &http.Client{}

	url := mysite.siteURL + "/Groupes/ecouteAudioGroup/" + strconv.Itoa(roomID) + "/" + token + "/" + strconv.Itoa(on)

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

		ok, httpErr := strconv.Atoi(string(body))
		if ok != 1 {
			return fmt.Errorf("body error : '%s' on URL '%s' \r\n", string(body), url)
		}
	}

	return nil
}
