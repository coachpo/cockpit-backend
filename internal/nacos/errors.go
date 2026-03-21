package nacos

import "errors"

var ErrStaticMode = errors.New("static mode: mutations disabled, configure NACOS_ADDR to enable")
