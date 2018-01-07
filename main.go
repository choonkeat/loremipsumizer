package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

var inputFiles string
var outputPath string
var dictionaryFile string
var ignoreFile string
var ignoreContent []string

func main() {
	flag.StringVar(&inputFiles, "files", "", "space separated list of input files")
	flag.StringVar(&outputPath, "outdir", os.TempDir(), "path to write loremipsumized files")
	flag.Parse() // we want to use `outputPath`

	flag.StringVar(&dictionaryFile, "dictfile", path.Join(outputPath, "lorem.yml"), "(optional) dictionary of translations")
	flag.StringVar(&ignoreFile, "ignorefile", ".lorem.ignore", "(optional) dictionary of translations")
	flag.Parse()

	data, err := ioutil.ReadFile(ignoreFile)
	if err == nil {
		b := bufio.NewReader(bytes.NewReader(data))
		for {
			line, _, err := b.ReadLine()
			if err != nil {
				break
			}
			ignoreContent = append(ignoreContent, string(line))
		}
	}
	log.Printf("ignoreContent = %#v", ignoreContent)

	dict, fn := dictionary(dictionaryFile)
	defer fn()
	if dict == nil {
		return
	}

	for _, s := range extractInputfiles(inputFiles) {
		if err := loremipsumize(s, dict, path.Join(outputPath, path.Base(s))); err != nil {
			log.Println(err.Error())
			break
		}
	}
}

func extractInputfiles(str string) []string {
	parts := strings.Split(str, " ")
	max := len(parts)
	for i := max; i >= 0; i-- {
		filename := strings.Join(parts[0:i], " ")
		if _, err := os.Stat(filename); err == nil {
			return append([]string{filename}, extractInputfiles(strings.Join(parts[i:max], " "))...)
		}
	}
	return []string{}
}

func dictionary(filename string) (map[string]string, func()) {
	dict := map[string]string{}
	callbackFn := func() {
		data, err := yaml.Marshal(dict)
		if err != nil {
			return
		}
		ioutil.WriteFile(filename, data, 0600)
		log.Println("wrote dictionary", dictionaryFile, err)
	}

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return dict, callbackFn
	}
	yaml.Unmarshal(data, &dict)

	return dict, callbackFn
}

func loremipsumize(inputFile string, dict map[string]string, outputFile string) error {
	log.Printf("[loremipsumize] read %s", inputFile)
	f, err := os.Open(inputFile)
	if err != nil {
		return err
	}
	defer f.Close()

	v := map[string]interface{}{}
	json.NewDecoder(f).Decode(&v)

	v2, err := loremipsumizeMap(v, dict, []string{"."})
	if err != nil {
		return err
	}

	f, err = os.Create(outputFile)
	if err == nil {
		defer f.Close()
		err = json.NewEncoder(f).Encode(v2)
		log.Printf("[loremipsumize] wrote %s %#v", outputFile, err)
	}
	return err
}

func loremipsumizeFloat64(v float64, dict map[string]string, nesting []string) (interface{}, error) {
	s := strings.TrimSuffix(fmt.Sprintf("%f", v), ".000000")
	s2, err := loremipsumizeString(s, dict, nesting)
	if err != nil {
		return v, err
	}
	return strconv.ParseFloat(s2.(string), 64)
}

var characterSetRegexp = map[string]*regexp.Regexp{
	"abcdefghijklmnopqrstuvwxyz": regexp.MustCompile(`[a-z]+`),
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ": regexp.MustCompile(`[A-Z]+`),
	"0123456789":                 regexp.MustCompile(`[0-9]+`),
}

func loremipsumizeString(v string, dict map[string]string, nesting []string) (interface{}, error) {
	var data = []byte(v)
	for characterSets, re := range characterSetRegexp {
		data = re.ReplaceAllFunc(data, func(old []byte) []byte {
			if isIgnored(fmt.Sprintf("%#v", string(old))) {
				return old
			}

			new := make([]byte, len(old))
			for i := len(old) - 1; i >= 0; i-- {
				new[i] = byte(characterSets[rand.Intn(len(characterSets))])
			}
			dict[string(old)] = string(new)
			return new
		})
	}
	return string(data), nil
}

func loremipsumizeArray(input []interface{}, dict map[string]string, nesting []string) (interface{}, error) {
	output := []interface{}{}
	for _, v := range input {
		v2, err := loremipsumizeAny(v, dict, nesting)
		output = append(output, v2)
		if err != nil {
			return output, err
		}
	}
	return output, nil
}

func loremipsumizeAny(v interface{}, dict map[string]string, nesting []string) (interface{}, error) {
	switch v.(type) {
	case nil:
		return v, nil
	case float64:
		return loremipsumizeFloat64(v.(float64), dict, nesting)
	case string:
		return loremipsumizeString(v.(string), dict, nesting)
	case []interface{}:
		return loremipsumizeArray(v.([]interface{}), dict, nesting)
	case map[string]interface{}:
		return loremipsumizeMap(v.(map[string]interface{}), dict, nesting)
	default:
		log.Fatalf("unknown type %#v", v)
		return nil, nil
	}
}

func loremipsumizeMap(input map[string]interface{}, dict map[string]string, nesting []string) (map[string]interface{}, error) {
	output := map[string]interface{}{}
	for k, v := range input {
		currNesting := append(nesting, k)
		if isIgnored(strings.Join(currNesting, "/")) {
			output[k] = v
			continue
		}
		v2, err := loremipsumizeAny(v, dict, currNesting)
		if err != nil {
			return output, err
		}
		output[k] = v2
	}

	return output, nil
}

func isIgnored(line string) bool {
	for _, phrase := range ignoreContent {
		if strings.HasSuffix(line, phrase) {
			return true
		}
	}
	return false
}
