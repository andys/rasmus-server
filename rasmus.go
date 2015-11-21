package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
)

type Request struct {
	Uuid    string
	Command string
	Path    string
	Mode    os.FileMode
	Input   string
	Params  []string
}

type Response struct {
	Completed bool
	Success   bool
	Output    string
	Msg       string
	At        int64
	uuid      string
}

type Rasmus struct {
	Whitelist       []string
	ResponseChannel chan Response
	RedisInput      redis.Conn
	RedisOutput     redis.Conn
	timeout         time.Duration
	namespace       string
	host            string
	password        string
}

func main() {
	app := Rasmus{timeout: 10 * time.Second, ResponseChannel: make(chan Response, 20)}
	if len(os.Args) > 1 {
		app.host = os.Args[1]
	} else {
		app.host = "127.0.0.1:6379"
  }
	if len(os.Args) > 2 {
		app.password = os.Args[2]
	}
	if len(os.Args) > 3 {
		app.namespace = os.Args[2]
	}

	go app.ResponseSender()
	for {
		app.ProcessOneRequest()
	}
}

func (app *Rasmus) ResponseSender() {
	log("ResponseSender: listening")
	for {
		response := <-app.ResponseChannel
		app.SendOneResponse(response)
	}
}

func (app *Rasmus) SendOneResponse(response Response) {
	json := response.Encode()
	unsent := true
	for unsent {
		if app.RedisOutput == nil {
			app.RedisOutput = app.dial()
		}
		key := app.redisKey("resp:" + response.uuid)
		_, err := app.RedisOutput.Do("LPUSH", key, json)
		if err != nil {
			app.RedisOutput = nil
			log("Error in redis LPUSH: " + err.Error())
			time.Sleep(app.timeout)
		} else {
			unsent = false
			app.RedisOutput.Do("EXPIRE", key, "600")
		}
	}
}

func (app *Rasmus) redisKey(keyappend string) string {
	keyparts := []string{app.namespace, "rasmus", keyappend}
	if keyparts[0] == "" {
		keyparts = keyparts[1:]
	}
	return strings.Join(keyparts, ":")
}

func (app *Rasmus) dial() (conn redis.Conn) {
	for conn == nil {
		var err error
		conn, err = redis.DialTimeout("tcp4", app.host, app.timeout, app.timeout, app.timeout)
		if conn == nil {
			log("Error connecting to redis " + app.host + ": " + err.Error())
			time.Sleep(app.timeout)
		} else {
			log("Connected to redis " + app.host)
			if app.password != "" {
				_, err = conn.Do("AUTH", app.password)
				if err != nil {
					log("Error during AUTH:" + err.Error())
				}
			}
		}
	}
	return conn
}

func (app *Rasmus) ProcessOneRequest() {
	if app.RedisInput == nil {
		app.RedisInput = app.dial()
	}

	rawRequest, err := app.RedisInput.Do("BRPOP", app.redisKey("req"), 5)
	requestStrings, err := redis.Strings(rawRequest, err)
	if err == nil {
		var request Request
		err := json.Unmarshal([]byte(requestStrings[1]), &request)
		if err == nil {
			go app.Handle(request)
		} else {
			log("Error unmarshalling request: " + err.Error())
		}
	} else if err != redis.ErrNil {
		app.RedisInput = nil // we got an error, so try reconnecting
		log("Error in redis BRPOP: " + err.Error())
		time.Sleep(app.timeout)
	}

}

func (app *Rasmus) Handle(req Request) {
	resp := Response{uuid: req.Uuid, Msg: "OK"}
	log(fmt.Sprintf("[%s] %q(%q)", req.Uuid, req.Command, req.Path))

	if req.Command == "read" {
		byteArray, err := ioutil.ReadFile(req.Path)
		resp.Output = string(byteArray[:])
		if err != nil {
			resp.Msg = err.Error()
		} else {
			resp.Completed = true
		}

	} else if req.Command == "write" {
		err := ioutil.WriteFile(req.Path, []byte(req.Input), req.Mode)
		if err != nil {
			resp.Msg = err.Error()
		} else {
			resp.Completed = true
		}

	} else if req.Command == "list" {
		fileinfo, err := os.Stat(req.Path)
		if err != nil {
			resp.Msg = "Error: " + err.Error()
		} else {
			resp.Completed = true
			resp.Msg = fmt.Sprintf("%d %d %d", fileinfo.Size(), fileinfo.Mode(), fileinfo.ModTime().Unix())
		}

	} else if req.Command == "execute" {
		shellCmd := exec.Command(req.Path, req.Params...)

		if req.Input != "" {
			stdin, _ := shellCmd.StdinPipe()
			stdin.Write([]byte(req.Input))
			stdin.Close()
		}

		byteArray, err := shellCmd.CombinedOutput()
		resp.Output = string(byteArray[:])
		if err != nil {
			resp.Msg = err.Error()
		}
		if shellCmd.ProcessState != nil && shellCmd.ProcessState.Exited() {
			resp.Completed = true
			resp.Success = shellCmd.ProcessState.Success()
		}

	} else {
		resp.Msg = "Unknown command " + req.Command
	}
	log(fmt.Sprintf("[%s] result: %t %s", resp.uuid, resp.Completed, resp.Msg))
	resp.At = time.Now().Unix()
	app.ResponseChannel <- resp
}

func (resp *Response) Encode() []byte {
	result, err := json.Marshal(resp)
	if err != nil {
		log("Error marshalling JSON: " + err.Error())
		return []byte("{}")
	}
	return result
}

func log(logstring string) {
	fmt.Println(logstring)
}
