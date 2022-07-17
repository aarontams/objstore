// Copyright (c) The Thanos Authors.
// Licensed under the Apache License 2.0.

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/thanos-io/objstore/providers/oci"

	"github.com/fatih/structtag"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/thanos-io/objstore/client"
	"github.com/thanos-io/objstore/providers/azure"
	"github.com/thanos-io/objstore/providers/bos"
	"github.com/thanos-io/objstore/providers/cos"
	"github.com/thanos-io/objstore/providers/filesystem"
	"github.com/thanos-io/objstore/providers/gcs"
	"github.com/thanos-io/objstore/providers/oss"
	"github.com/thanos-io/objstore/providers/s3"
	"github.com/thanos-io/objstore/providers/swift"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
)

var (
	configs        map[string]interface{}
	possibleValues []string

	bucketConfigs = map[client.ObjProvider]interface{}{
		client.AZURE:      azure.Config{},
		client.GCS:        gcs.Config{},
		client.S3:         s3.DefaultConfig,
		client.SWIFT:      swift.DefaultConfig,
		client.COS:        cos.DefaultConfig,
		client.ALIYUNOSS:  oss.Config{},
		client.FILESYSTEM: filesystem.Config{},
		client.BOS:        bos.Config{},
		client.OCI:        oci.Config{},
	}
)

func init() {
	configs = map[string]interface{}{}

	for typ, config := range bucketConfigs {
		configs[name(config)] = client.BucketConfig{Type: typ, Config: config}
	}

	for k := range configs {
		possibleValues = append(possibleValues, k)
	}
}

func name(typ interface{}) string {
	return fmt.Sprintf("%T", typ)
}

func main() {
	app := kingpin.New(filepath.Base(os.Args[0]), "Thanos config examples generator.")
	app.HelpFlag.Short('h')
	structName := app.Flag("name", fmt.Sprintf("Name of the struct to generated example for. Possible values: %v", strings.Join(possibleValues, ","))).Required().String()

	errLogger := level.Error(log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr)))
	if _, err := app.Parse(os.Args[1:]); err != nil {
		errLogger.Log("err", err)
		os.Exit(1)
	}

	if c, ok := configs[*structName]; ok {
		if err := generate(c, os.Stdout); err != nil {
			errLogger.Log("err", err)
			os.Exit(1)
		}
		return
	}

	errLogger.Log("err", errors.Errorf("%v struct not found. Possible values %v", *structName, strings.Join(possibleValues, ",")))
	os.Exit(1)
}

func generate(obj interface{}, w io.Writer) error {
	// We forbid omitempty option. This is for simplification for doc generation.
	if err := checkForOmitEmptyTagOption(obj); err != nil {
		return errors.Wrap(err, "invalid type")
	}
	return yaml.NewEncoder(w).Encode(obj)
}

func checkForOmitEmptyTagOption(obj interface{}) error {
	return checkForOmitEmptyTagOptionRec(reflect.ValueOf(obj))
}

func checkForOmitEmptyTagOptionRec(v reflect.Value) error {
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			tags, err := structtag.Parse(string(v.Type().Field(i).Tag))
			if err != nil {
				return errors.Wrapf(err, "%s: failed to parse tag %q", v.Type().Field(i).Name, v.Type().Field(i).Tag)
			}

			tag, err := tags.Get("yaml")
			if err != nil {
				return errors.Wrapf(err, "%s: failed to get tag %q", v.Type().Field(i).Name, v.Type().Field(i).Tag)
			}

			for _, opts := range tag.Options {
				if opts == "omitempty" {
					return errors.Errorf("omitempty is forbidden for config, but spotted on field '%s'", v.Type().Field(i).Name)
				}
			}

			if err := checkForOmitEmptyTagOptionRec(v.Field(i)); err != nil {
				return errors.Wrapf(err, "%s", v.Type().Field(i).Name)
			}
		}

	case reflect.Ptr:
		return errors.New("nil pointers are not allowed in configuration")

	case reflect.Interface:
		return checkForOmitEmptyTagOptionRec(v.Elem())
	}

	return nil
}
