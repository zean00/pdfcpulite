/*
Copyright 2018 The pdfcpu Authors.

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

package filter

import (
	"bytes"
	"fmt"
	"io"
	"strconv"

	"errors"

	"github.com/hhrutter/lzw"
)

type lzwDecode struct {
	baseFilter
}

// Encode implements encoding for an LZWDecode filter.
func (f lzwDecode) Encode(r io.Reader) (*bytes.Buffer, error) {

	fmt.Println("EncodeLZW begin")

	var b bytes.Buffer

	ec, ok := f.parms["EarlyChange"]
	if !ok {
		ec = 1
	}

	wc := lzw.NewWriter(&b, ec == 1)
	defer wc.Close()

	written, err := io.Copy(wc, r)
	if err != nil {
		return nil, err
	}
	fmt.Printf("EncodeLZW end: %d bytes written\n", written)

	return &b, nil
}

// Decode implements decoding for an LZWDecode filter.
func (f lzwDecode) Decode(r io.Reader) (*bytes.Buffer, error) {

	fmt.Println("DecodeLZW begin")

	p, found := f.parms["Predictor"]
	if found && p > 1 {
		return nil, errors.New("DecodeLZW: unsupported predictor %d" + strconv.Itoa(p))
	}

	ec, ok := f.parms["EarlyChange"]
	if !ok {
		ec = 1
	}

	rc := lzw.NewReader(r, ec == 1)
	defer rc.Close()

	var b bytes.Buffer
	written, err := io.Copy(&b, rc)
	if err != nil {
		return nil, err
	}
	fmt.Printf("DecodeLZW: decoded %d bytes.\n", written)

	return &b, nil
}
