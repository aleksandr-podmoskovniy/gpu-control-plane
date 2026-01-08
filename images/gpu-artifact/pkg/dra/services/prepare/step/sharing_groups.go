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

package step

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	configapi "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/configapi"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

type timeSlicingGroup struct {
	config       *configapi.TimeSlicingConfig
	reqIndexes   []int
	stateIndexes []int
	deviceUUIDs  []string
	deviceIndex  map[int]int
}

func (g *timeSlicingGroup) add(reqIdx, stateIdx int, uuid string) {
	if g.deviceIndex == nil {
		g.deviceIndex = map[int]int{}
	}
	g.deviceIndex[stateIdx] = len(g.deviceUUIDs)
	g.reqIndexes = append(g.reqIndexes, reqIdx)
	g.stateIndexes = append(g.stateIndexes, stateIdx)
	g.deviceUUIDs = append(g.deviceUUIDs, uuid)
}

type mpsGroup struct {
	key          string
	config       *configapi.MpsConfig
	reqIndexes   []int
	stateIndexes []int
	deviceUUIDs  []string
	deviceIndex  map[int]int
	existing     *domain.PreparedMpsState
}

func ensureMpsGroup(groups map[string]*mpsGroup, deviceType string, cfg *configapi.MpsConfig) *mpsGroup {
	key := fmt.Sprintf("%s/%s", strings.ToLower(deviceType), mpsConfigKey(cfg))
	group := groups[key]
	if group == nil {
		group = &mpsGroup{key: key, config: cfg}
		groups[key] = group
	}
	return group
}

func (g *mpsGroup) add(reqIdx, stateIdx int, uuid string) {
	if g.deviceIndex == nil {
		g.deviceIndex = map[int]int{}
	}
	g.deviceIndex[stateIdx] = len(g.deviceUUIDs)
	g.reqIndexes = append(g.reqIndexes, reqIdx)
	g.stateIndexes = append(g.stateIndexes, stateIdx)
	g.deviceUUIDs = append(g.deviceUUIDs, uuid)
}

func mergeExistingMpsState(group *mpsGroup, sharing *domain.PreparedSharing) error {
	if group == nil || sharing == nil || sharing.MPS == nil {
		return nil
	}
	if group.existing == nil {
		group.existing = sharing.MPS
		return nil
	}
	if group.existing.ControlID != sharing.MPS.ControlID {
		return fmt.Errorf("conflicting MPS control IDs %q and %q", group.existing.ControlID, sharing.MPS.ControlID)
	}
	return nil
}

func mpsConfigKey(cfg *configapi.MpsConfig) string {
	if cfg == nil {
		return "default"
	}
	payload, err := json.Marshal(cfg)
	if err != nil {
		return "default"
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])[:8]
}

func buildMpsControlID(claimUID, groupKey string, deviceUUIDs []string) string {
	sort.Strings(deviceUUIDs)
	payload := strings.Join(append([]string{claimUID, groupKey}, deviceUUIDs...), "|")
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%s-%s", claimUID, hex.EncodeToString(sum[:])[:6])
}
