package parse

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseName(t *testing.T) {
	testCases := []struct {
		name      string
		raw       string
		floorCode string
		expected  ParsedName
		expectErr bool
	}{
		{
			name:      "Standard Case 1",
			raw:       "东3#2-3",
			floorCode: "2",
			expected:  ParsedName{Dorm: "东3", Floor: 2, Seq: 3},
			expectErr: false,
		},
		{
			name:      "Standard Case 2",
			raw:       "北村E3-1",
			floorCode: "3",
			expected:  ParsedName{Dorm: "北村E", Floor: 3, Seq: 1},
			expectErr: false,
		},
		{
			name:      "Standard Case 3",
			raw:       "延英楼2F",
			floorCode: "2",
			expected:  ParsedName{Dorm: "延英楼", Floor: 2, Seq: 0},
			expectErr: false,
		},
		{
			name:      "Name with spaces",
			raw:       "西区 5# 10-1",
			floorCode: "10",
			expected:  ParsedName{Dorm: "西区 5", Floor: 10, Seq: 1},
			expectErr: false,
		},
		{
			name:      "Multiple Hashes",
			raw:       "南##1-1",
			floorCode: "1",
			expected:  ParsedName{Dorm: "南", Floor: 1, Seq: 1},
			expectErr: false,
		},
		{
			name:      "No Hash",
			raw:       "中楼5-12",
			floorCode: "5",
			expected:  ParsedName{Dorm: "中楼", Floor: 5, Seq: 12},
			expectErr: false,
		},
		{
			name:      "Wrong Floor Code",
			raw:       "延英楼2F",
			floorCode: "1",
			expected:  ParsedName{Dorm: "延英楼", Floor: 2, Seq: 0},
			expectErr: false,
		},
		{
			name:      "Fallback with floorCode",
			raw:       "DormWithNoFloorSeq",
			floorCode: "8",
			expected:  ParsedName{Dorm: "DormWithNoFloorSeq", Floor: 8, Seq: 0},
			expectErr: false,
		},
		{
			name:      "Parsing Failure",
			raw:       "InvalidName",
			floorCode: "",
			expectErr: true,
		},
		{
			name:      "Parsing Failure with invalid floorcode",
			raw:       "InvalidName",
			floorCode: "A",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ParseName(tc.raw, tc.floorCode)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, parsed)
			}
		})
	}
}
