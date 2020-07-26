package executor

import "errors"

var (
	LOGRUS_PROCESS_ERROR_PANIC_FIELD_KEY = "LOGRUS_PROCESS_ERROR_PANIC_FIELD_KEY"

	ERR_PROCESS_RETRY             = errors.New("command need retry")
	ERR_PROCESS_PARSE_DOC_FAILURE = errors.New("parse response doc failure")
)
