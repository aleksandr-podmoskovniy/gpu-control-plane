/*
Copyright 2025 Flant JSC

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

package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	endpoint := flag.String("endpoint", "http://localhost:9090/metrics", "metrics endpoint exposed by kube-api-rewriter")
	timeout := flag.Duration("timeout", 10*time.Second, "HTTP client timeout")
	flag.Parse()

	client := &http.Client{Timeout: *timeout}
	req, err := http.NewRequest(http.MethodGet, *endpoint, nil)
	if err != nil {
		log.Fatalf("build request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("unexpected status code: %s", resp.Status)
	}

	if _, err := io.Copy(os.Stdout, resp.Body); err != nil {
		log.Fatalf("read response: %v", err)
	}

	fmt.Println()
}
