
Tiny server written in Go that receives commands to run, runs them, and posts the result, all via Redis.

### Command request
(`LPUSH rasmusReq`)
```JSON
{
    "uuid":"11111",
 "command":"execute",
    "path":"echo",
  "params":["hi","there"]
}
```


### Command response
(`HGET rasmusResp 1111`)
```JSON
{
 "Completed":true,
   "Success":true,
    "Output":"hi there\n",
       "Msg":"OK",
        "At":1408789746
}
```


### Supported commands (and parameters)

* read (`path`)
* write (`input` to `path` with `mode`)
* execute (`input` piped to `path` with `params`)
* list (`path`)
