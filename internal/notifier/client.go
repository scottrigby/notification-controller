/*
Copyright 2020 The Flux CD contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

func postMessage(address string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling notification payload failed: %w", err)
	}

	b := bytes.NewBuffer(data)

	req, err := http.NewRequest("POST", address, b)
	if err != nil {
		return fmt.Errorf("http.NewRequest failed: %w", err)
	}
	req.Header.Set("Content-type", "application/json")

	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()

	res, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("sending notification failed: %w", err)
	}

	defer res.Body.Close()
	statusCode := res.StatusCode
	if statusCode != 200 {
		body, _ := ioutil.ReadAll(res.Body)
		return fmt.Errorf("sending notification failed: %s", string(body))
	}

	return nil
}
