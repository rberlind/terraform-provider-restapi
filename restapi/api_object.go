package restapi

import (
  "log"
  "errors"
  "fmt"
  "encoding/json"
  "bytes"
  "github.com/davecgh/go-spew/spew"
  "strings"
)

type api_object struct {
  api_client           *api_client
  get_path             string
  post_path            string
  put_path             string
  delete_path          string
  debug                bool
  id                   string

  /* Set internally */
  data         map[string]interface{} /* Data as managed by the user */
  api_data     map[string]interface{} /* Data as available from the API */
}

// Make an api_object to manage a RESTful object in an API
func NewAPIObject (i_client *api_client, i_get_path string, i_post_path string, i_put_path string, i_delete_path string, i_id string, i_data string, i_debug bool) (*api_object, error) {
  if i_debug {
    log.Printf("api_object.go: Constructing debug api_object\n")
    log.Printf(" id: %s\n", i_id)
  }

  obj := api_object{
    api_client: i_client,
    get_path: i_get_path,
    post_path: i_post_path,
    put_path: i_put_path,
    delete_path: i_delete_path,
    debug: i_debug,
    id: i_id,
    data: make(map[string]interface{}),
    api_data: make(map[string]interface{}),
  }

  if "" == i_get_path    { return nil, errors.New("No GET path passed to api_object constructor") }
  if "" == i_post_path   { return nil, errors.New("No POST path passed to api_object constructor") }
  if "" == i_put_path    { return nil, errors.New("No PUT path passed to api_object constructor") }
  if "" == i_delete_path { return nil, errors.New("No DELETE path passed to api_object constructor") }
  if "" == i_data        { return nil, errors.New("No data passed to api_object constructor") }

  if i_data != "" {
    if i_debug { log.Printf("api_object.go: Parsing data: '%s'", i_data) }

    err := json.Unmarshal([]byte(i_data), &obj.data)
    if err != nil {
      return nil, err
    }

    /* Opportunistically set the object's ID if it is provided in the data.
       If it is not set, we will get it later in synchronize_state */
    if obj.id == "" {
      val, ok := obj.data[obj.api_client.id_attribute]
      if ok {
        obj.id = fmt.Sprintf("%v", val)
      } else if !obj.api_client.write_returns_object && !obj.api_client.create_returns_object && obj.api_client.id_header == "" {
        /* If the id is not set and we cannot obtain it later, error out to be safe */
        return nil, errors.New(fmt.Sprintf("Provided data does not have %s attribute for the object's id and the client is not configured to read the object or its id from a POST response. Without an id, the object cannot be managed.", obj.api_client.id_attribute))
      }
    }
  }

  if obj.debug { log.Printf("api_object.go: Constructed object: %s", obj.toString()) }
  return &obj, nil
}

// Convert the important bits about this object to string representation
// This is useful for debugging.
func (obj *api_object) toString() string {
  var buffer bytes.Buffer
  buffer.WriteString(fmt.Sprintf("id: %s\n", obj.id))
  buffer.WriteString(fmt.Sprintf("get_path: %s\n", obj.get_path))
  buffer.WriteString(fmt.Sprintf("post_path: %s\n", obj.post_path))
  buffer.WriteString(fmt.Sprintf("put_path: %s\n", obj.put_path))
  buffer.WriteString(fmt.Sprintf("delete_path: %s\n", obj.delete_path))
  buffer.WriteString(fmt.Sprintf("debug: %t\n", obj.debug))
  buffer.WriteString(fmt.Sprintf("data: %s\n", spew.Sdump(obj.data)))
  buffer.WriteString(fmt.Sprintf("api_data: %s\n", spew.Sdump(obj.api_data)))
  return buffer.String()
}

/* Centralized function to ensure that our data as managed by
   the api_object is updated with data that has come back from
   the API */
func (obj *api_object) update_state(state string) error {
  if obj.debug { log.Printf("api_object.go: Updating API object state to '%s'\n", state) }

  /* Other option - Decode as JSON Numbers instead of golang datatypes
  d := json.NewDecoder(strings.NewReader(res_body))
  d.UseNumber()
  err = d.Decode(&obj.api_data)
  */
  var err error
  if state != "" {
    err = json.Unmarshal([]byte(state), &obj.api_data)
    if err != nil { return err }
  }

  /* A usable ID was not passed (in constructor or here),
     so we have to guess what it is from the data structure */
  if obj.id == "" {
    val, ok := obj.api_data[obj.api_client.id_attribute]
    if ok {
      /* Coax to string */
      obj.id = fmt.Sprintf("%v", val)
      log.Printf("api_object.go: Updating object id (unset) to '%s'\n", obj.id)
    } else {
      /* An ID is REQUIRED to manage the object. We canot proceed */
      err_message := fmt.Sprintf("api_object.go: Error: %s is not in the data presented nor passed in the constructor.\n", obj.api_client.id_attribute)
      err_message += fmt.Sprintf("List of keys available:\n")
      for k := range obj.data { err_message += fmt.Sprintf("  %s\n", k) }
      errors.New(err_message)
    }
  } else if obj.debug {
    log.Printf("api_object.go: Not updating id. It is already set to '%s'\n", obj.id)
  }

  /* Any keys that come from the data we want to copy are done here */
  if len(obj.api_client.copy_keys) > 0  && obj.api_data != nil {
    for _, key := range obj.api_client.copy_keys {
      if obj.debug {
        log.Printf("api_object.go: Copying key '%s' from api_data (%v) to data (%v)\n", key, obj.api_data[key], obj.data[key])
      }
      obj.data[key] = obj.api_data[key]
    }
  } else if obj.debug {
    log.Printf("api_object.go: copy_keys or api_data is empty - not attempting to copy data")
  }

  if obj.debug {
    log.Printf("api_object.go: final object after synchronization of state:\n%+v\n", obj.toString())
  }
  return err
}

func (obj *api_object) create_object() error {
  /* Failsafe: The constructor should prevent this situation, but
     protect here also. If no id is set, and the API does not respond
     with the id of whatever gets created, we have no way to know what
     the object's id will be. Abandon this attempt */
  if obj.id == "" && !obj.api_client.write_returns_object && !obj.api_client.create_returns_object && obj.api_client.id_header == "" {
    return errors.New("ERROR: Provided object does not have an id set and the client is not configured to read the object after a POST or PUT response, possibly after retrieving the ID from a header of the POST response. Without an id, the object cannot be managed.")
  }

  b, _ := json.Marshal(obj.data)
  res_headers, res_body, err := obj.api_client.send_request("POST", strings.Replace(obj.post_path, "{id}", obj.id, -1), string(b))
  if err != nil { return err }

  // Try to get ID from id_header if it was specified.
  var id_header string
  if obj.api_client.id_header != "" {
    for name, headers := range res_headers {
      if name == obj.api_client.id_header {
        for _, h := range headers {
          if obj.api_client.id_header_is_url {
            // parse ID from last segment of URL
            h = strings.TrimSuffix(h, "/")
            segments := strings.Split(h, "/")
            if len(segments) > 0 {
              id_header = segments[len(segments)-1]
              log.Printf("api_object.go: found ID '%s' in URL in header '%s'\n", id_header, obj.api_client.id_header)
            } else {
              id_header = ""
              log.Printf("api_object.go: id_header '%s' was empty or not a URL\n", obj.api_client.id_header)
            }
          } else {
            // use entire header
            log.Printf("api_object.go: found ID '%s' in header '%s' using entire header\n", id_header, obj.api_client.id_header)
            id_header = h
          }
        }
      }
    }
    if id_header != "" {
      obj.id = id_header
      log.Printf("api_object.go: Setting obj.id to '%s'\n", id_header)
    } else {
      log.Printf("api_object.go: id_header '%s' was empty or not found\n", obj.api_client.id_header)
    }
  }

  /* We will need to sync state as well as get the object's ID */
  if obj.api_client.write_returns_object || obj.api_client.create_returns_object {
    if obj.debug {
      log.Printf("api_object.go: Parsing response from POST to update internal structures (write_returns_object=%t, create_returns_object=%t)...\n",
        obj.api_client.write_returns_object, obj.api_client.create_returns_object)
    }
    err = obj.update_state(res_body)
    /* Yet another failsafe. In case something terrible went wrong internally,
       bail out so the user at least knows that the ID did not get set. */
    if obj.id == "" { return errors.New("Internal validation failed. Object ID is not set, but *may* have been created. This should never happen!") }
  } else {
    if obj.debug {
      log.Printf("api_object.go: Requesting created object from API (write_returns_object=%t, create_returns_object=%t)...\n",
        obj.api_client.write_returns_object, obj.api_client.create_returns_object)
    }
    err = obj.read_object()
  }
  return err
}

func (obj *api_object) read_object() error {
  if obj.id == "" {
    return errors.New("Cannot read an object unless the ID has been set.")
  } else {
    log.Printf("api_object.go: Invoking read_object() with existing ID:\n")
  }

  res_headers, res_body, err := obj.api_client.send_request("GET", strings.Replace(obj.get_path, "{id}", obj.id, -1), "")
  if err != nil { return err }

  if obj.debug {
    log.Printf("api_object.go: Response headers:\n")
    for name, headers := range res_headers {
      for _, h := range headers {
       log.Printf("api_object.go:   %v: %v", name, h)
      }
    }
  }

  err = obj.update_state(res_body)
  return err
}

func (obj *api_object) update_object() error {
  if obj.id == "" {
    return errors.New("Cannot update an object unless the ID has been set.")
  }

  b, _ := json.Marshal(obj.data)
  res_headers, res_body, err := obj.api_client.send_request("PUT", strings.Replace(obj.put_path, "{id}", obj.id, -1), string(b))
  if err != nil { return err }

  if obj.debug {
    log.Printf("api_object.go: Response headers:\n")
    for name, headers := range res_headers {
      for _, h := range headers {
       log.Printf("api_object.go:   %v: %v", name, h)
      }
    }
  }

  if obj.api_client.write_returns_object {
    if obj.debug { log.Printf("api_object.go: Parsing response from PUT to update internal structures (write_returns_object=true)...\n") }
    err = obj.update_state(res_body)
  } else {
    if obj.debug { log.Printf("api_object.go: Requesting updated object from API (write_returns_object=false)...\n") }
    err = obj.read_object()
  }
  return err
}

func (obj *api_object) delete_object() error {
  if obj.id == "" {
    log.Printf("api_object.go: WARNING: Attempting to delete an object that has no id set. Assuming this is OK.\n")
    return nil
  }

  _, _, err := obj.api_client.send_request("DELETE", strings.Replace(obj.delete_path, "{id}", obj.id, -1), "")
  if err != nil { return err }

  return nil
}
