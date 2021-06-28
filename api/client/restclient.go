/*Copyright 2020 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.*/
package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	//"infinibox-csi-driver/helper"

	resty "github.com/go-resty/resty/v2"
	"k8s.io/klog"
)

type HostConfig struct {
	ApiHost  string
	UserName string
	Password string
}
type Resultmetadata struct {
	NoOfObject int `json:"number_of_objects,omitempty"`
	TotalPages int `json:"pages_total,omitempty"`
	Page       int `json:"page,omitempty"`
	PageSize   int `json:"page_size,omitempty"`
}
type ApiResponse struct {
	Result   interface{}    `json:"result,omitempty"`
	MetaData Resultmetadata `json:"metadata,omitempty"`
	Error    interface{}    `json:"error,omitempty"`
}

var rClient *resty.Client

//NewRestClient : Initialize http client
func NewRestClient() (*restclient, error) {
	if rClient == nil {
		rClient = resty.New()
		rClient.SetHeader("Content-Type", "application/json")
		rClient.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
		rClient.SetDisableWarn(true)
		rClient.SetTimeout(60 * time.Second)
	}
	return &restclient{}, nil
}

//RestClient : implement to make rest client
type RestClient interface {
	Get(ctx context.Context, url string, hostconfig HostConfig, expectedResp interface{}) (interface{}, error)
	Post(ctx context.Context, url string, hostconfig HostConfig, body, expectedResp interface{}) (interface{}, error)
	Put(ctx context.Context, url string, hostconfig HostConfig, body, expectedResp interface{}) (interface{}, error)
	Delete(ctx context.Context, url string, hostconfig HostConfig) (interface{}, error)
	GetWithQueryString(ctx context.Context, url string, hostconfig HostConfig, queryString string, expectedResp interface{}) (interface{}, error)
}

type restclient struct {
	RestClient
	SecretMap map[string]string
}

// Get :
func (rc *restclient) Get(ctx context.Context, url string, hostconfig HostConfig, expectedResp interface{}) (interface{}, error) {
	klog.V(2).Infof("called client.Get with url %s ", url)
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			klog.Errorf("error in Get() while making request on %s url error : %v ", url, err)
			err = errors.New("error in Get() " + fmt.Sprint(res))
		}
	}()
	if err := checkHttpClient(); err != nil {
		klog.Errorf("checkHttpClient returned err %v ", err)
		return nil, err
	}
	response, err := rClient.SetHostURL(hostconfig.ApiHost).
		SetBasicAuth(hostconfig.UserName, hostconfig.Password).R().Get(url)
	resp, err := rc.checkResponse(response, err, expectedResp)
	if err != nil {
		klog.Errorf("error in validating response %v", err)
		return nil, err
	}
	klog.V(2).Infof("client.Get request completed.")
	return resp, err
}

func (rc *restclient) GetWithQueryString(ctx context.Context, url string, hostconfig HostConfig, queryString string, expectedResp interface{}) (interface{}, error) {
	klog.V(2).Infof("called client.GetWithQueryString for api %s and querystring is %s ", url, queryString)
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			klog.Errorf("error in GetWithQueryString while making request on %s url error : %v ", url, err)
			err = errors.New("error in GetWithQueryString " + fmt.Sprint(res))
		}
	}()
	if err := checkHttpClient(); err != nil {
		klog.Errorf("checkHttpClient returned err %v  ", err)
		return nil, err
	}
	response, err := rClient.SetHostURL(hostconfig.ApiHost).
		SetBasicAuth(hostconfig.UserName, hostconfig.Password).
		R().SetQueryString(queryString).Get(url)

	res, err := rc.checkResponse(response, err, expectedResp)
	if err != nil {
		klog.Errorf("error in validating response %v ", err)
		return nil, err
	}
	klog.V(2).Infof("GetWithQueryString request completed.")
	return res, err
}

func (rc *restclient) Post(ctx context.Context, url string, hostconfig HostConfig, body, expectedResp interface{}) (interface{}, error) {
	klog.V(2).Infof("called Post with url %s", url)
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			klog.Errorf("error in Post while making request on %s url error : %v ", url, err)
			err = errors.New("error in Post " + fmt.Sprint(res))
		}
	}()
	if err := checkHttpClient(); err != nil {
		klog.Errorf("checkHttpClient returned err %v  ", err)
		return nil, err
	}
	response, err := rClient.SetHostURL(hostconfig.ApiHost).
		SetBasicAuth(hostconfig.UserName, hostconfig.Password).R().
		SetBody(body).
		Post(url)
	res, err := rc.checkResponse(response, err, expectedResp)
	if err != nil {
		klog.Errorf("error in validating response %v ", err)
		return nil, err
	}
	klog.V(2).Infof("Post request completed.")
	return res, err
}

func (rc *restclient) Put(ctx context.Context, url string, hostconfig HostConfig, body, expectedResp interface{}) (interface{}, error) {
	klog.V(2).Infof("Put: context.Context '%s'", ctx)
	klog.V(2).Infof("Put: url '%s'", url)
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			klog.Errorf("error in Put while making request on %s url error : %v ", url, err)
			err = errors.New("error in Put " + fmt.Sprint(res))
		}
	}()
	if err := checkHttpClient(); err != nil {
		klog.Errorf("checkHttpClient returned err %v ", err)
		return nil, err
	}
	response, err := rClient.SetHostURL(hostconfig.ApiHost).
		SetBasicAuth(hostconfig.UserName, hostconfig.Password).
		R().SetBody(body).Put(url)
	res, err := rc.checkResponse(response, err, expectedResp)
	if err != nil {
		klog.Errorf("error in validating response %v ", err)
		return nil, err
	}
	klog.V(2).Infof("client.Put request Completed")
	return res, err
}

func (rc *restclient) Delete(ctx context.Context, url string, hostconfig HostConfig) (interface{}, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			klog.Errorf("error in Delete while making request on %s url error : %v ", url, err)
			err = errors.New("error in Delete " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("called client.Delete with url %s  ", url)
	if err := checkHttpClient(); err != nil {
		klog.Errorf("checkHttpClient returned err %v ", err)
		return nil, err
	}
	response, err := rClient.SetHostURL(hostconfig.ApiHost).
		SetBasicAuth(hostconfig.UserName, hostconfig.Password).
		R().Delete(url)
	res, err := rc.checkResponse(response, err, nil)
	if err != nil {
		klog.Errorf("error in validating response %v ", err)
		return nil, err
	}
	klog.V(2).Infof("client.Delete request Completed")
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
func (rc *restclient) checkResponse(res *resty.Response, err error, resptpye interface{}) (result ApiResponse, er error) {
	defer func() {
		if recovered := recover(); recovered != nil && er == nil {
			er = errors.New("error while parsing management api response " + fmt.Sprint(recovered) + "for request " + res.Request.URL)
		}
	}()

	if res.StatusCode() == http.StatusUnauthorized {
		return result, errors.New("Request authentication failed for : " + res.Request.URL)
	}

	if res.StatusCode() == http.StatusServiceUnavailable {
		return result, errors.New(res.Status())
	}

	if err != nil {
		klog.Errorf("Error While Resty call for request " + res.Request.URL + err.Error())
		return result, err
	}
	if resptpye != nil {
		// start: bind to given struct type
		apiresp := ApiResponse{}
		apiresp.Result = resptpye
		if err := json.Unmarshal(res.Body(), &apiresp); err != nil {
			klog.Errorf("checkResponse expected type provided case. err %v", err)
			return result, er
		}
		if res != nil {
			if str, iserr := rc.parseError(apiresp.Error); iserr {
				return result, errors.New(str)
			}
			if apiresp.Result != nil {
				return apiresp, nil
			} else {
				return result, errors.New("result part of response is nil for request " + res.Request.URL)
			}
		} else {
			return result, errors.New("empty response for " + res.Request.URL)
		}
		// end: bind to given struct
	} else {
		klog.V(2).Infof("checkResponse resptpye nil case ", resptpye)
		var response interface{}
		if er := json.Unmarshal(res.Body(), &response); er != nil {
			klog.Errorf("checkResponse expected type provided case. error %v", er)
			return result, er
		}

		if res != nil {
			responseinmap := response.(map[string]interface{})
			if responseinmap != nil {
				if str, iserr := rc.parseError(responseinmap["error"]); iserr {
					return result, errors.New(str)
				}
				result.Result = responseinmap["result"]
				result.Error = responseinmap["error"]
				if result.Result != nil {
					return result, nil
				} else {
					return result, errors.New("result part of response is nil for request " + res.Request.URL)
				}
			} else {
				return result, errors.New("empty response for " + res.Request.URL)
			}
		} else {
			return result, errors.New("empty response for " + res.Request.URL)
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

// // Pretty print a struct, map, array or slice variable. Write using klog.V(4).Infof().
// // Copied here from helper/ because of a cyclic import error.
// func prettyKlogDebug(msg string, v interface{}) (err error) {
// 	b, err := json.MarshalIndent(v, "", "  ")
// 	if err == nil {
// 		klog.V(4).Infof("%s %s", msg, string(b))
// 	}
// 	return
// }
