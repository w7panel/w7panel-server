package procpath

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const OverflowID uint32 = 65534

type IDMapper struct {
	uidRanges []idMapRange
	gidRanges []idMapRange
}

type idMapRange struct {
	containerID uint32
	hostID      uint32
	size        uint32
}

func NewIDMapper(pid, subPid string) *IDMapper {
	uidMapPath, gidMapPath := GetIDMapPaths(pid, subPid)
	return NewIDMapperFromPaths(uidMapPath, gidMapPath)
}

func NewIDMapperFromPaths(uidMapPath, gidMapPath string) *IDMapper {
	return &IDMapper{
		uidRanges: readIDMap(uidMapPath),
		gidRanges: readIDMap(gidMapPath),
	}
}

func GetIDMapPaths(pid, subPid string) (string, string) {
	basePath := filepath.Join(GetBasePath(), pid)
	if subPid != "" && subPid != "0" {
		basePath = filepath.Join(GetRootPath(pid), "proc", subPid)
	}
	return filepath.Join(basePath, "uid_map"), filepath.Join(basePath, "gid_map")
}

func (m *IDMapper) HostToContainerUID(uid uint32) uint32 {
	mapped, ok := mapHostToContainer(m.uidRanges, uid)
	if !ok {
		return OverflowID
	}
	return mapped
}

func (m *IDMapper) HostToContainerGID(gid uint32) uint32 {
	mapped, ok := mapHostToContainer(m.gidRanges, gid)
	if !ok {
		return OverflowID
	}
	return mapped
}

func (m *IDMapper) ContainerToHostUID(uid uint32) uint32 {
	mapped, _ := m.TryContainerToHostUID(uid)
	return mapped
}

func (m *IDMapper) ContainerToHostGID(gid uint32) uint32 {
	mapped, _ := m.TryContainerToHostGID(gid)
	return mapped
}

func (m *IDMapper) TryContainerToHostUID(uid uint32) (uint32, bool) {
	return mapContainerToHost(m.uidRanges, uid)
}

func (m *IDMapper) TryContainerToHostGID(gid uint32) (uint32, bool) {
	return mapContainerToHost(m.gidRanges, gid)
}

func readIDMap(path string) []idMapRange {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	ranges := parseIDMap(string(data))
	if len(ranges) == 0 {
		return nil
	}
	return ranges
}

func parseIDMap(data string) []idMapRange {
	lines := strings.Split(data, "\n")
	ranges := make([]idMapRange, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		containerID, err := strconv.ParseUint(fields[0], 10, 32)
		if err != nil {
			continue
		}
		hostID, err := strconv.ParseUint(fields[1], 10, 32)
		if err != nil {
			continue
		}
		size, err := strconv.ParseUint(fields[2], 10, 32)
		if err != nil || size == 0 {
			continue
		}
		ranges = append(ranges, idMapRange{
			containerID: uint32(containerID),
			hostID:      uint32(hostID),
			size:        uint32(size),
		})
	}
	return ranges
}

func mapHostToContainer(ranges []idMapRange, id uint32) (uint32, bool) {
	if ranges == nil {
		return id, true
	}
	for _, item := range ranges {
		if id >= item.hostID && id-item.hostID < item.size {
			return item.containerID + (id - item.hostID), true
		}
	}
	return 0, false
}

func mapContainerToHost(ranges []idMapRange, id uint32) (uint32, bool) {
	if ranges == nil {
		return id, true
	}
	for _, item := range ranges {
		if id >= item.containerID && id-item.containerID < item.size {
			return item.hostID + (id - item.containerID), true
		}
	}
	return 0, false
}
