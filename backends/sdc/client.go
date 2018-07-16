package sdc

import (
	"errors"
	"io/ioutil"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/kelseyhightower/confd/backends/etcd"
	"github.com/kelseyhightower/confd/backends/etcdv3"
	"github.com/BurntSushi/toml"
)

// The StoreClient interface is implemented by objects that can retrieve
// key/value pairs from a backend store.
type StoreClient interface {
	GetValues(keys []string) (map[string]string, error)
	WatchPrefix(prefix string, keys []string, waitIndex uint64, stopChan chan bool) (uint64, error)
}

type SdcClient struct {
	sdcClient StoreClient
	backend   string
}

type Scope struct {
	System string `toml:"SYSTEM_NAME"`
	Local  string `toml:"NODE_NAME"`
}

var SuStatus bool
var SdcScope Scope

func CheckSuStatus(sdcClient StoreClient, systemName string, nodeName string){
        suKey := make([]string, 0, 20)
        suKey = append(suKey, "/" + systemName + "_SU_status")
        suStatusMap, err := sdcClient.GetValues(suKey)
        if err != nil {
            SuStatus = false
        }
        
        suValue, ok := suStatusMap["/" + systemName + "_SU_status"]
	if ok{
            if strings.Contains(suValue,"\""+ nodeName + "\""){
               SuStatus = true
            }else{
               SuStatus = false
            }
        }else{
            SuStatus = false 
        }
}
// New is used to create a storage client based on our configuration.
func NewSdcClient(machines []string, cert, key, caCert string, basicAuth bool, username string, password string) (StoreClient, error) {

	str, err := exec.Command("sdcctl", "--version").Output()
	if err != nil {
		return nil, err
	}

	reg := regexp.MustCompile(`\d+.\d+-\d+`)
	sdcVersion := reg.FindString(string(str))

	sdcVersionSlice := strings.Split(sdcVersion, "-")
	mainVersion, err := strconv.ParseFloat(sdcVersionSlice[0], 64)
	if err != nil {
		return nil, err
	}
	minorVersion, err := strconv.Atoi(sdcVersionSlice[1])
	if err != nil {
		return nil, err
	}

	var client SdcClient
	switch {
	case mainVersion < 1.4:
		// Create the etcd client upfront and use it for the life of the process.
		// The etcdClient is an http.Client and designed to be reused.
		sdcClient, err := etcd.NewEtcdClient(machines, cert, key, caCert, basicAuth, username, password)
		client.sdcClient = sdcClient
		client.backend = "etcd"
		return client, err
	case mainVersion == 1.4 && minorVersion < 23:
		sdcClient, err := etcd.NewEtcdClient(machines, cert, key, caCert, basicAuth, username, password)
		client.sdcClient = sdcClient
		client.backend = "etcd"
		return client, err
	default:
		sdcClient, err := etcdv3.NewEtcdClient(machines, cert, key, caCert, basicAuth, username, password)
		client.sdcClient = sdcClient
		client.backend = "etcdv3"
		return client, err
	}

	return nil, errors.New("Invalid backend")
}

func (c SdcClient) GetValues(keys []string) (map[string]string, error) {

	clientConf, err := ioutil.ReadFile("/etc/etcd/etcd.client.conf")
	if err != nil {
		return nil, err
	}

	_, err = toml.Decode(string(clientConf), &SdcScope)

	if err != nil {
		return nil, err
	}

        SuStatus = false 
        CheckSuStatus(c.sdcClient, SdcScope.System, SdcScope.Local)
	switch c.backend {
	case "etcdv3":
		scopeKey := make([]string, 0, 20)
		for _, key := range keys {
			keydir := strings.Split(key, "/")
			key = ""
			for _, dir := range keydir {
				if dir == SdcScope.System || dir == "" {
					continue
				}
				key = key + "/" + dir
			}
                        if key == ""{
                            if SuStatus{
                                scopeKey = append(scopeKey, "/" + SdcScope.System + "_SU/")
                            }else{
                                scopeKey = append(scopeKey, "/" + SdcScope.System + "/")
                            }
                        }else{
			    keypath := strings.Split(key, "/")
                            if keypath[1] == "config"{
                                 if SuStatus{
                                     scopeKey = append(scopeKey, "/"+SdcScope.System+"_SU"+key)
			             scopeKey = append(scopeKey, "/"+SdcScope.System+"_SU/"+SdcScope.Local+key)
                                 }else{
                                     scopeKey = append(scopeKey, "/"+SdcScope.System+key)
			             scopeKey = append(scopeKey, "/"+SdcScope.System+"/"+SdcScope.Local+key)
                                 }
                            }else if keypath[1] == "services"{
                                     scopeKey = append(scopeKey, "/"+SdcScope.System+key)
			             scopeKey = append(scopeKey, "/"+SdcScope.System+"/"+SdcScope.Local+key)
                            }else{
                                 if SuStatus{
                                     scopeKey = append(scopeKey,"/"+SdcScope.System+"_SU/config"+key)
                                     scopeKey = append(scopeKey,"/"+SdcScope.System+"/services"+key)
                                     scopeKey = append(scopeKey,"/"+SdcScope.System+"_SU/"+SdcScope.Local+"/config"+key)
                                     scopeKey = append(scopeKey,"/"+SdcScope.System+"/"+SdcScope.Local+"/services"+key)      
                                 }else{
                                     scopeKey = append(scopeKey,"/"+SdcScope.System+"/config"+key)
                                     scopeKey = append(scopeKey,"/"+SdcScope.System+"/services"+key)
                                     scopeKey = append(scopeKey,"/"+SdcScope.System+"/"+SdcScope.Local+"/config"+key)
                                     scopeKey = append(scopeKey,"/"+SdcScope.System+"/"+SdcScope.Local+"/services"+key)
                             }                 
                           }
                        }                         
		}
		return c.sdcClient.GetValues(scopeKey)

	case "etcd":
		result := make(map[string]string)
		systemKey := make([]string, 1)
		localKey := make([]string, 1)
		systemConfigKey := make([]string, 1)
		systemServicesKey := make([]string, 1)
		localConfigKey := make([]string, 1)
		localServicesKey := make([]string, 1)
		for _, key := range keys {

	            keydir := strings.Split(key, "/")
	            key = "" 
		    for _, dir := range keydir {
			if dir == SdcScope.System || dir == "" {
				continue
			}
			key = key + "/" + dir
		    } 
                    if key == ""{
                        if SuStatus{
                            systemKey[0] = "/" + SdcScope.System +"_SU/"
                        }else{
                            systemKey[0] = "/" + SdcScope.System +"/"
                        }
                        systemVar, err := c.sdcClient.GetValues(systemKey)
                        if err == nil {
                                  for k, v := range systemVar{
                                      result[k] = v
                                  }                                  
                         }
                    }else{    
			keypath := strings.Split(key, "/")
                        if keypath[1] == "config" || keypath[1] == "services"{                           
                             if SuStatus && keypath[1] == "config"{
			         systemKey[0] = "/" + SdcScope.System +"_SU"+ key
			         localKey[0] = "/" + SdcScope.System + "_SU/" + SdcScope.Local + key
                             }else{
			         systemKey[0] = "/" + SdcScope.System + key
			         localKey[0] = "/" + SdcScope.System + "/" + SdcScope.Local + key 
                             }
		             systemVar, err := c.sdcClient.GetValues(systemKey)
			     if err == nil {
                                  for k, v := range systemVar{
                                      result[k] = v
                                  }
			     }
			     localVar, err := c.sdcClient.GetValues(localKey)
			     if err == nil {
                                  for k, v := range localVar{
                                      result[k] = v
                                  } 
			     }
                        }else{
                             if SuStatus{
                                 systemConfigKey[0] = "/" + SdcScope.System + "_SU/config" + key
                                 systemServicesKey[0] = "/" + SdcScope.System + "/services" + key
			         localConfigKey[0] = "/" + SdcScope.System + "_SU/" + SdcScope.Local +"/config" + key
			         localServicesKey[0] = "/" + SdcScope.System + "/" + SdcScope.Local +"/services" + key
                             }else{
                                 systemConfigKey[0] = "/" + SdcScope.System + "/config" + key
                                 systemServicesKey[0] = "/" + SdcScope.System + "/services" + key
			         localConfigKey[0] = "/" + SdcScope.System + "/" + SdcScope.Local +"/config" + key
			         localServicesKey[0] = "/" + SdcScope.System + "/" + SdcScope.Local +"/services" + key
                             }
		             systemConfigVar, err := c.sdcClient.GetValues(systemConfigKey)
			     if err == nil {
                                  for k, v := range systemConfigVar{
                                      result[k] = v
                                  }
			     }

		             systemServicesVar, err := c.sdcClient.GetValues(systemServicesKey)
			     if err == nil {
                                  for k, v := range systemServicesVar{
                                      result[k] = v
                                  }
			     }

			     localConfigVar, err := c.sdcClient.GetValues(localConfigKey)
			     if err == nil {
                                  for k, v := range localConfigVar{
                                      result[k] = v
                                  }
			     }

			     localServicesVar, err := c.sdcClient.GetValues(localServicesKey)
			     if err == nil {
                                  for k, v := range localServicesVar{
                                      result[k] = v
                                  }
			     }

                        }
                    } 

		}
		return result, nil

	}
        return nil, errors.New("Invalid backend")
}

func (c SdcClient) WatchPrefix(prefix string, keys []string, waitIndex uint64, stopChan chan bool) (uint64, error) {
	// return something > 0 to trigger a key retrieval from the store
	return c.sdcClient.WatchPrefix(prefix, keys, waitIndex, stopChan)
}
