package parse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	seqRe   = regexp.MustCompile(`-(\d+)\s*$`)
	floorRe = regexp.MustCompile(`(?i)(\d+)\s*(?:F|层)?\s*$`)
)

// ParsedName holds the structured data parsed from a machine's display name.
type ParsedName struct {
	Dorm  string
	Floor int
	Seq   int
}

// ParseName extracts dorm, floor, and sequence number from a raw machine name string.
// func ParseName(raw string, floorCode string) (ParsedName, error) {
// 	s := strings.TrimSpace(raw)

// 	// 1. Primary parsing strategy: split by "-"
// 	parts := strings.Split(s, "-")
// 	if len(parts) == 2 {
// 		seqStr := strings.TrimSpace(parts[1])
// 		nameAndFloor := strings.TrimSpace(parts[0])

// 		seq, errSeq := strconv.Atoi(seqStr)
// 		floorMatches := floorRe.FindStringSubmatch(nameAndFloor)

// 		if errSeq == nil && len(floorMatches) == 2 {
// 			floor, errFloor := strconv.Atoi(floorMatches[1])
// 			if errFloor == nil {
// 				dorm := strings.TrimSpace(strings.TrimSuffix(nameAndFloor, floorMatches[0]))
// 				dorm = strings.ReplaceAll(dorm, "#", "")
// 				return ParsedName{Dorm: dorm, Floor: floor, Seq: seq}, nil
// 			}
// 		}
// 	}

// 	// 2. Fallback: Use floorCode if available
// 	if floorCode != "" {
// 		if f, err := strconv.Atoi(floorCode); err == nil {
// 			// As a last resort, assume the whole name is the dorm name
// 			// and seq is unknown (0). Remove hash marks.
// 			dorm := strings.ReplaceAll(s, "#", "")
// 			// Also remove the floor code if it appears as a suffix in the name
// 			// This handles cases where the name might be "Dorm3" and floorcode is "3"
// 			dorm = strings.TrimSuffix(dorm, floorCode)
// 			return ParsedName{Dorm: dorm, Floor: f, Seq: 0}, nil
// 		}
// 	}

// 	return ParsedName{}, fmt.Errorf("unable to parse name: %q", raw)
// }

// 单机楼默认序号：按你的业务设定；要是保持“未知=0”也行
const defaultSeqForSingleMachine = 1

// ParseName extracts dorm, floor, and sequence number from a raw machine name string.
func ParseName(raw string, floorCode string) (ParsedName, error) {
	// 0) 预处理：把 # 当成“分隔符”而不是删除，以免数字粘连
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, "#", " ")
	// 压缩多余空白（含中文空格等）
	s = strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(s, " "))

	// 1) 先取可选的 "-编号"（在末尾），并把整段移除
	seq := 0
	if loc := seqRe.FindStringSubmatchIndex(s); loc != nil {
		// loc: [fullStart, fullEnd, group1Start, group1End]
		if n, err := strconv.Atoi(s[loc[2]:loc[3]]); err == nil {
			seq = n
			s = strings.TrimSpace(s[:loc[0]]) // 去掉 "-编号" 整段
		}
	}

	// 2) 从主体末尾取楼层，并把这段从楼名里删掉
	floor := 0
	dorm := s
	if loc := floorRe.FindStringSubmatchIndex(s); loc != nil {
		if n, err := strconv.Atoi(s[loc[2]:loc[3]]); err == nil {
			floor = n
			dorm = strings.TrimSpace(s[:loc[0]]) // 去掉楼层尾巴
		}
	}

	// 3) 主体没取到楼层时，用 floorCode 兜底，并把相应尾巴去掉
	if floor == 0 && floorCode != "" {
		if f, err := strconv.Atoi(floorCode); err == nil {
			floor = f
			// 删除与 floorCode 匹配的结尾（如 "8" / "8F" / "8层"）
			tailRe := regexp.MustCompile(`(?i)\s*` + regexp.QuoteMeta(floorCode) + `\s*(?:F|层)?\s*$`)
			dorm = strings.TrimSpace(tailRe.ReplaceAllString(dorm, ""))
		}
	}

	// 4) temp fix: 栋 -> 东
	dorm = strings.ReplaceAll(dorm, "栋", "东")

	if floor == 0 {
		return ParsedName{}, fmt.Errorf("unable to parse floor from name: %q", raw)
	}

	// 5) 没有显式编号就保持为 0
	return ParsedName{Dorm: dorm, Floor: floor, Seq: seq}, nil
}
