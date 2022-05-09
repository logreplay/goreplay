package ip

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey"
	"gopkg.in/go-playground/assert.v1"
)

func TestUnit_GetClientIp(t *testing.T) {
	type testcase struct {
		IP        string
		ExpectIP  string
		Mask      string
		err       error
		errExpect error
	}
	var err1 = errors.New("test")
	testcases := []testcase{
		{"1.1.1.1", "1.1.1.1", "255.255.255.0", nil, nil},
		{"1.1.1.1", "", "255.255.255.0", err1, err1},
		{"", "", "", nil, errors.New("can not find the client ip address")},
	}
	for _, tc := range testcases {
		gomonkey.ApplyFunc(net.InterfaceAddrs, func() ([]net.Addr, error) {
			if tc.IP == "" {
				return []net.Addr{}, tc.err
			}
			iplist := strings.Split(tc.IP, ".")
			item1, _ := strconv.Atoi(iplist[0])
			item2, _ := strconv.Atoi(iplist[1])
			item3, _ := strconv.Atoi(iplist[2])
			item4, _ := strconv.Atoi(iplist[3])
			return []net.Addr{
				&net.IPNet{IP: net.IPv4(uint8(item1), uint8(item2), uint8(item3), uint8(item4)), Mask: []byte(tc.Mask)},
			}, tc.err
		})
		ip, err := GetClientIP()
		assert.Equal(t, ip, tc.ExpectIP)
		assert.Equal(t, err, tc.errExpect)
	}
}
