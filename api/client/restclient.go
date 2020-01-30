package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	log "github.com/sirupsen/logrus"

	resty "github.com/go-resty/resty/v2"
	//"k8s.io/kubernetes/pkg/util/strings"
)

const (
	// USERNAME = "INFINIBOX_USER"
	// PASSWORD = "INFINIBOX_PASS"
	// APIHOST  = "INFINIBOX_HOST"
	USERNAME = "admin"
	PASSWORD = "123456"
	APIHOST  = "https://172.17.35.61/"
)

type apiresponse struct {
	Result   interface{} `json:"result,omitempty"`
	MetaData interface{} `json:"metadata,omitempty"`
	Error    interface{} `json:"error,omitempty"`
}

var rClient *resty.Client

func NewRestClient() (*restclient, error) {
	if rClient == nil {
		rClient = resty.New()
		rClient.SetHeader("Content-Type", "application/json")
		rClient.SetBasicAuth(USERNAME, PASSWORD)
		rClient.SetHostURL(APIHOST)
		//rClient.SetBasicAuth(os.Getenv(USERNAME), os.Getenv(PASSWORD)) // or SetResult(AuthSuccess{})
		rClient.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
		//rClient.SetHostURL(os.Getenv(APIHOST))
		rClient.SetDisableWarn(true)

		//validation for the URL
		_, err := rClient.R().Get(os.Getenv(APIHOST))
		if err != nil {
			return nil, err
			//log.Error("error in validating URL ",err)
		}

	}
	return &restclient{}, nil
}

// implement to make rest client
type RestClient interface {
	Get(ctx context.Context, url string, headerMap map[string]string, expectedResp interface{}) (interface{}, error)
	Post(ctx context.Context, url string, headerMap map[string]string, body, expectedResp interface{}) (interface{}, error)
	Put(ctx context.Context, url string, headerMap map[string]string, body, expectedResp interface{}) (interface{}, error)
	Delete(ctx context.Context, url string, headerMap map[string]string) (interface{}, error)
	GetWithQueryString(ctx context.Context, url, queryString string, expectedResp interface{}) (interface{}, error)
}

type restclient struct {
	RestClient
	//Resty resty.Client
}

//

func (rc *restclient) Get(ctx context.Context, url string, headerMap map[string]string, expectedResp interface{}) (interface{}, error) {
	log.Debugf("called client.Get with url %s ", url)
	if err := checkHttpClient(); err != nil {
		log.Debugf("checkHttpClient returned err %v ", err)
		return nil, err
	}
	response, err := rClient.R().SetHeaders(headerMap).Get(url)
	log.Debugf("client.Get returned err %v for url %s ", err, url)
	resp, err := rc.checkResponse(response, err, expectedResp)
	if err != nil {
		log.Debugf("error in validating response %v", err)
		return nil, err
	}
	log.Debug("client.Get request completed.")
	return resp, err
}

func (rc *restclient) GetWithQueryString(ctx context.Context, url, queryString string, expectedResp interface{}) (interface{}, error) {
	log.Debugf("called client.GetWithQueryString for api %s and querystring is %s ", url, queryString)
	if err := checkHttpClient(); err != nil {
		log.Debugf("checkHttpClient returned err %v  ", err)
		return nil, err
	}
	response, err := rClient.R().SetQueryString(queryString).Get(url)
	res, err := rc.checkResponse(response, err, expectedResp)
	if err != nil {
		log.Debugf("error in validating response %v ", err)
		return nil, err
	}
	log.Debug("GetWithQueryString request completed.")
	return res, err
}

func (rc *restclient) Post(ctx context.Context, url string, headerMap map[string]string, body, expectedResp interface{}) (interface{}, error) {
	log.Debugf("called client.Post with url %s", url)
	if err := checkHttpClient(); err != nil {
		log.Debugf("checkHttpClient returned err %v  ", err)
		return nil, err
	}
	response, err := rClient.R().
		SetBody(body).
		SetHeaders(headerMap).
		Post(url)
	log.Debugf("resty Post err %v  ", err)
	res, err := rc.checkResponse(response, err, expectedResp)
	if err != nil {
		log.Debug("error in validating response %v ", err)
		return nil, err
	}
	log.Debug("Post request completed.")
	return res, err
}

func (rc *restclient) Put(ctx context.Context, url string, headerMap map[string]string, body, expectedResp interface{}) (interface{}, error) {
	log.Debugf("called client.Put with url %s  ", url)
	if err := checkHttpClient(); err != nil {
		log.Debugf("checkHttpClient returned err %v ", err)
		return nil, err
	}
	response, err := rClient.R().
		SetBody(body).
		SetHeaders(headerMap).
		Put(url)
	res, err := rc.checkResponse(response, err, expectedResp)
	if err != nil {
		log.Debugf("error in validating response %v ", err)
		return nil, err
	}
	log.Debug("client.Put request Completed")
	return res, err
}

func (rc *restclient) Delete(ctx context.Context, url string, headerMap map[string]string) (interface{}, error) {
	log.Debugf("called client.Delete with url %s  ", url)
	if err := checkHttpClient(); err != nil {
		log.Debugf("checkHttpClient returned err %v ", err)
		return nil, err
	}
	response, err := rClient.R().
		SetHeaders(headerMap).
		Delete(url)
	res, err := rc.checkResponse(response, err, nil)
	if err != nil {
		log.Debugf("error in validating response %v ", err)
		return nil, err
	}
	log.Debug("client.Delete request Completed")
	return res, err
}

func checkHttpClient() error {
	if rClient == nil {
		_, err := NewRestClient()
		return err
	}
	return nil
}

//Method to check the response is valid or not
func (rc *restclient) checkResponse(res *resty.Response, err error, resptpye interface{}) (result interface{}, er error) {
	defer func() {
		if recovered := recover(); recovered != nil && er == nil {
			er = errors.New("error while parsing management api response " + fmt.Sprint(recovered) + "for request " + res.Request.URL)
		}
	}()

	if res.StatusCode() == http.StatusUnauthorized {
		return nil, errors.New("Request authentication failed for : " + res.Request.URL)
	}

	if res.StatusCode() == http.StatusServiceUnavailable {
		return nil, errors.New(res.Status())
	}

	if err != nil {
		log.Error("Error While Resty call for request " + res.Request.URL + err.Error())
		return nil, err
	}
	if resptpye != nil {
		// start: bind to given struct type
		apiresp := apiresponse{}
		apiresp.Result = resptpye
		if err := json.Unmarshal(res.Body(), &apiresp); err != nil {
			log.Errorf("checkResponse expected type provided case. err %v", err)
			return nil, er
		}
		if res != nil {
			if str, iserr := rc.parseError(apiresp.Error); iserr {
				return nil, errors.New(str)
			}
			if apiresp.Result != nil {
				return apiresp.Result, nil
			} else {
				return nil, errors.New("result part of response is nil for request " + res.Request.URL)
			}
		} else {
			return nil, errors.New("empty response for " + res.Request.URL)
		}
		// end: bind to given struct
	} else {
		log.Debug("checkResponse resptpye nil case ", resptpye)
		var response interface{}
		if er := json.Unmarshal(res.Body(), &response); er != nil {
			log.Errorf("checkResponse expected type provided case. error %v", er)
			return nil, er
		}

		if res != nil {
			responseinmap := response.(map[string]interface{})
			if responseinmap != nil {

				if str, iserr := rc.parseError(responseinmap["error"]); iserr {
					return nil, errors.New(str)
				}
				if result := responseinmap["result"]; result != nil {
					return responseinmap["result"], nil
				} else {
					return nil, errors.New("result part of response is nil for request " + res.Request.URL)
				}
			} else {
				return nil, errors.New("empty response for " + res.Request.URL)
			}
		} else {
			return nil, errors.New("empty response for " + res.Request.URL)
		}
	}
}

//Method to check error response from management api
func (rc *restclient) parseError(responseinmap interface{}) (str string, iserr bool) {
	defer func() {
		if res := recover(); res != nil {
			str = "recovered in parseError  " + fmt.Sprint(res)
			iserr = true
		}

	}()

	if responseinmap != nil {
		resultmap := responseinmap.(map[string]interface{})
		return resultmap["code"].(string) + " " + resultmap["message"].(string), true
	}
	return "", false
}
