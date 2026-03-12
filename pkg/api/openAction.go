/*
	Copyright 2024 The pdfcpu Authors.

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

package api

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ListOpenAction lists rs's OpenAction.
func ListOpenAction(rs io.ReadSeeker, conf *model.Configuration) ([]string, error) {
	if rs == nil {
		return nil, errors.New("pdfcpu: ListOpenAction: missing rs")
	}

	if conf == nil {
		conf = model.NewDefaultConfiguration()
	} else {
		conf.ValidationMode = model.ValidationRelaxed
	}
	conf.Cmd = model.LISTOPENACTION

	ctx, err := ReadAndValidate(rs, conf)
	if err != nil {
		return nil, err
	}

	o, found := ctx.RootDict.Find("OpenAction")
	if !found || o == nil {
		return []string{"No open action set"}, nil
	}

	o, err = ctx.Dereference(o)
	if err != nil {
		return nil, err
	}

	switch o := o.(type) {
	case types.Array:
		return []string{fmt.Sprintf("Destination: %s", o)}, nil
	case types.Dict:
		s, _ := o.Find("S")
		js, _ := o.Find("JS")
		return []string{fmt.Sprintf("Action: S=%v JS=%v", s, js)}, nil
	default:
		return []string{fmt.Sprintf("OpenAction: %v", o)}, nil
	}
}

// ListOpenActionFile lists inFile's OpenAction.
func ListOpenActionFile(inFile string, conf *model.Configuration) ([]string, error) {
	f, err := os.Open(inFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return ListOpenAction(f, conf)
}

// SetOpenAction sets rs's OpenAction and writes the result to w.
// dest is a destination string: "page:fit" where fit is one of:
//
//	Fit, FitB, FitH:top, FitV:left, FitBH:top, FitBV:left,
//	XYZ:left:top:zoom, FitR:left:bottom:right:top
func SetOpenAction(rs io.ReadSeeker, w io.Writer, dest string, conf *model.Configuration) error {
	if rs == nil {
		return errors.New("pdfcpu: SetOpenAction: missing rs")
	}

	if conf == nil {
		conf = model.NewDefaultConfiguration()
	} else {
		conf.ValidationMode = model.ValidationRelaxed
	}
	conf.Cmd = model.SETOPENACTION

	ctx, err := ReadAndValidate(rs, conf)
	if err != nil {
		return err
	}

	a, err := parseOpenActionDest(ctx, dest)
	if err != nil {
		return err
	}

	ctx.RootDict["OpenAction"] = a

	return Write(ctx, w, conf)
}

// SetOpenActionFile sets inFile's OpenAction and writes the result to outFile.
func SetOpenActionFile(inFile, outFile string, dest string, conf *model.Configuration) (err error) {
	var f1, f2 *os.File

	if f1, err = os.Open(inFile); err != nil {
		return err
	}

	tmpFile := inFile + ".tmp"
	if outFile != "" && inFile != outFile {
		tmpFile = outFile
	}
	if f2, err = os.Create(tmpFile); err != nil {
		f1.Close()
		return err
	}

	defer func() {
		if err != nil {
			f2.Close()
			f1.Close()
			os.Remove(tmpFile)
			return
		}
		if err = f2.Close(); err != nil {
			return
		}
		if err = f1.Close(); err != nil {
			return
		}
		if outFile == "" || inFile == outFile {
			err = os.Rename(tmpFile, inFile)
		}
	}()

	return SetOpenAction(f1, f2, dest, conf)
}

// ResetOpenAction removes rs's OpenAction and writes the result to w.
func ResetOpenAction(rs io.ReadSeeker, w io.Writer, conf *model.Configuration) error {
	if rs == nil {
		return errors.New("pdfcpu: ResetOpenAction: missing rs")
	}

	if conf == nil {
		conf = model.NewDefaultConfiguration()
	} else {
		conf.ValidationMode = model.ValidationRelaxed
	}
	conf.Cmd = model.RESETOPENACTION

	ctx, err := ReadAndValidate(rs, conf)
	if err != nil {
		return err
	}

	delete(ctx.RootDict, "OpenAction")

	return Write(ctx, w, conf)
}

// ResetOpenActionFile removes inFile's OpenAction and writes the result to outFile.
func ResetOpenActionFile(inFile, outFile string, conf *model.Configuration) (err error) {
	var f1, f2 *os.File

	if f1, err = os.Open(inFile); err != nil {
		return err
	}

	tmpFile := inFile + ".tmp"
	if outFile != "" && inFile != outFile {
		tmpFile = outFile
	}
	if f2, err = os.Create(tmpFile); err != nil {
		f1.Close()
		return err
	}

	defer func() {
		if err != nil {
			f2.Close()
			f1.Close()
			os.Remove(tmpFile)
			return
		}
		if err = f2.Close(); err != nil {
			return
		}
		if err = f1.Close(); err != nil {
			return
		}
		if outFile == "" || inFile == outFile {
			err = os.Rename(tmpFile, inFile)
		}
	}()

	return ResetOpenAction(f1, f2, conf)
}

// parseOpenActionDest parses a destination string like "1:Fit" or "3:FitH:100" or "1:XYZ:0:0:1.5"
// and returns a PDF destination array.
func parseOpenActionDest(ctx *model.Context, dest string) (types.Array, error) {
	parts := strings.Split(dest, ":")
	if len(parts) < 2 {
		return nil, fmt.Errorf("pdfcpu: invalid open action dest %q, expected page:fitType[:params...]", dest)
	}

	pageNum, err := strconv.Atoi(parts[0])
	if err != nil || pageNum < 1 || pageNum > ctx.PageCount {
		return nil, fmt.Errorf("pdfcpu: invalid page number %q (must be 1-%d)", parts[0], ctx.PageCount)
	}

	pageIndRef, err := pageIndRefForNum(ctx, pageNum)
	if err != nil {
		return nil, err
	}

	fitType := parts[1]
	params := parts[2:]

	switch fitType {
	case "Fit", "FitB":
		return types.Array{*pageIndRef, types.Name(fitType)}, nil

	case "FitH", "FitV", "FitBH", "FitBV":
		a := types.Array{*pageIndRef, types.Name(fitType)}
		if len(params) >= 1 {
			v, err := parseNumOrNull(params[0])
			if err != nil {
				return nil, err
			}
			a = append(a, v)
		}
		return a, nil

	case "XYZ":
		a := types.Array{*pageIndRef, types.Name("XYZ")}
		for i := 0; i < 3; i++ {
			if i < len(params) {
				v, err := parseNumOrNull(params[i])
				if err != nil {
					return nil, err
				}
				a = append(a, v)
			}
		}
		return a, nil

	case "FitR":
		if len(params) < 4 {
			return nil, fmt.Errorf("pdfcpu: FitR requires 4 params (left, bottom, right, top), got %d", len(params))
		}
		a := types.Array{*pageIndRef, types.Name("FitR")}
		for i := 0; i < 4; i++ {
			v, err := parseNumOrNull(params[i])
			if err != nil {
				return nil, err
			}
			a = append(a, v)
		}
		return a, nil

	default:
		return nil, fmt.Errorf("pdfcpu: unknown fit type %q (valid: Fit, FitB, FitH, FitV, FitBH, FitBV, XYZ, FitR)", fitType)
	}
}

func parseNumOrNull(s string) (types.Object, error) {
	if s == "" || s == "null" {
		return nil, nil
	}
	if strings.Contains(s, ".") {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("pdfcpu: invalid number %q", s)
		}
		return types.Float(f), nil
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return nil, fmt.Errorf("pdfcpu: invalid number %q", s)
	}
	return types.Integer(i), nil
}

func pageIndRefForNum(ctx *model.Context, pageNum int) (*types.IndirectRef, error) {
	_, indRef, _, err := ctx.PageDict(pageNum, false)
	if err != nil {
		return nil, err
	}
	if indRef == nil {
		return nil, fmt.Errorf("pdfcpu: no indirect ref for page %d", pageNum)
	}
	return indRef, nil
}
