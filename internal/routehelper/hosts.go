package routehelper

import (
	"errors"
	"net/netip"
	"slices"
	"strings"
)

const (
	hostsBegin        = "# BEGIN Hello TSNet"
	hostsEnd          = "# END Hello TSNet"
	hostsAddedNewline = "# Hello TSNet added the preceding newline"
)

type HostEntry struct {
	Name    string
	Address netip.Addr
}

func CanonicalHostName(name string) string {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	if !validHostName(name) {
		return ""
	}
	return name
}

func validHostName(name string) bool {
	labels := strings.Split(name, ".")
	if len(labels) != 4 || labels[2] != "ts" || labels[3] != "net" {
		return false
	}
	for _, label := range labels[:2] {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
				return false
			}
		}
	}
	return true
}

func managedHosts(contents string, entries []HostEntry, routed map[netip.Addr]bool) (string, error) {
	start, end, offset := -1, -1, 0
	addedNewline := false
	for line := range strings.SplitAfterSeq(contents, "\n") {
		switch strings.TrimRight(line, "\r\n") {
		case hostsBegin:
			if start >= 0 {
				return "", errors.New("重复的 Hello TSNet hosts 开始标记")
			}
			start = offset
		case hostsEnd:
			if start < 0 || end >= 0 {
				return "", errors.New("意外的 Hello TSNet hosts 结束标记")
			}
			end = offset + len(line)
		case hostsAddedNewline:
			addedNewline = start >= 0 && end < 0 && offset == start+len(hostsBegin)+1
		}
		offset += len(line)
	}
	if start >= 0 && end < 0 {
		return "", errors.New("缺少 Hello TSNet hosts 结束标记")
	}
	original := contents
	if start >= 0 {
		if addedNewline && end == len(contents) && start > 0 && contents[start-1] == '\n' {
			start--
		}
		original = contents[:start] + contents[end:]
	}
	if len(entries) == 0 {
		return original, nil
	}
	existing := map[string]bool{
		"localhost.local": true, "broadcasthost.local": true, "local.local": true,
		"wpad.local": true, "isatap.local": true, "autodiscover.local": true,
	}
	for line := range strings.SplitSeq(original, "\n") {
		data, _, _ := strings.Cut(line, "#")
		fields := strings.Fields(data)
		if len(fields) > 1 {
			for _, name := range fields[1:] {
				existing[strings.ToLower(strings.TrimSuffix(name, "."))] = true
			}
		}
	}
	byName := make(map[string][]netip.Addr)
	shortOwner := make(map[string]string)
	for _, entry := range entries {
		name := entry.Name
		if !routed[entry.Address] || !validHostName(name) {
			continue
		}
		short, _, _ := strings.Cut(name, ".")
		if owner, ok := shortOwner[short]; ok && owner != name {
			shortOwner[short] = ""
		} else {
			shortOwner[short] = name
		}
		if !existing[name] {
			byName[name] = append(byName[name], entry.Address)
		}
	}
	if len(byName) == 0 {
		return original, nil
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	slices.Sort(names)
	var output strings.Builder
	output.WriteString(original)
	pad := original != "" && !strings.HasSuffix(original, "\n")
	if pad {
		output.WriteByte('\n')
	}
	output.WriteString(hostsBegin + "\n")
	if pad {
		output.WriteString(hostsAddedNewline + "\n")
	}
	for _, name := range names {
		short, _, _ := strings.Cut(name, ".")
		addrs := byName[name]
		slices.SortFunc(addrs, netip.Addr.Compare)
		for _, addr := range slices.Compact(addrs) {
			output.WriteString(addr.String() + "\t" + name)
			alias := short + ".local"
			if shortOwner[short] == name && !existing[alias] {
				output.WriteString(" " + alias)
			}
			output.WriteByte('\n')
		}
	}
	output.WriteString(hostsEnd + "\n")
	return output.String(), nil
}
