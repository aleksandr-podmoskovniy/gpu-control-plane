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

package promutil

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	invalidLabelCharRE = regexp.MustCompile(`[^a-zA-Z0-9_]`)
	matchAllCap        = regexp.MustCompile("([a-z0-9])([A-Z])")
)

// SkipLabel decides whether a label should be skipped.
type SkipLabel func(key, value string) bool

// WrapPrometheusLabels converts and wraps arbitrary labels to Prometheus-safe labels.
func WrapPrometheusLabels(labels map[string]string, prefix string, skip SkipLabel) map[string]string {
	keys, values := mapToPrometheusLabels(labels, prefix, skip)
	wrapLabels := make(map[string]string, len(keys))
	for i, k := range keys {
		wrapLabels[k] = values[i]
	}
	return wrapLabels
}

func mapToPrometheusLabels(labels map[string]string, prefix string, skip SkipLabel) ([]string, []string) {
	labelKeys := make([]string, 0, len(labels))
	labelValues := make([]string, 0, len(labels))

	sortedKeys := make([]string, 0)
	for key, value := range labels {
		if skip != nil && skip(key, value) {
			continue
		}
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	type conflictDesc struct {
		count   int
		initial int
	}

	conflicts := make(map[string]*conflictDesc)
	for _, k := range sortedKeys {
		labelKey := labelName(prefix, k)
		if conflict, ok := conflicts[labelKey]; ok {
			if conflict.count == 1 {
				labelKeys[conflict.initial] = labelConflictSuffix(labelKeys[conflict.initial], conflict.count)
			}

			conflict.count++
			labelKey = labelConflictSuffix(labelKey, conflict.count)
		} else {
			conflicts[labelKey] = &conflictDesc{
				count:   1,
				initial: len(labelKeys),
			}
		}
		labelKeys = append(labelKeys, labelKey)
		labelValues = append(labelValues, labels[k])
	}
	return labelKeys, labelValues
}

func labelName(prefix, labelName string) string {
	return prefix + "_" + lintLabelName(SanitizeLabelName(labelName))
}

// SanitizeLabelName replaces unsupported characters with underscores.
func SanitizeLabelName(s string) string {
	return invalidLabelCharRE.ReplaceAllString(s, "_")
}

func lintLabelName(s string) string {
	return toSnakeCase(s)
}

func toSnakeCase(s string) string {
	snake := matchAllCap.ReplaceAllString(s, "${1}_${2}")
	return strings.ToLower(snake)
}

func labelConflictSuffix(label string, count int) string {
	return fmt.Sprintf("%s_conflict%d", label, count)
}
