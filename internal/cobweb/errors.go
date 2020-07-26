package cobweb

import "errors"

type ProcessError struct {
	err string
}

func (p *ProcessError) Error() string {
	return p.err
}

func newProcessError(text string) ProcessError {
	return ProcessError{err: text}
}

var (
	// command method invoke retry method
	PROCESS_ERR_NEED_RETRY = newProcessError("command need retry")

	// command's Context or HTMLElement method which assert some doc struct failed
	// for example, HTML, ChildText
	PROCESS_ERR_PARSE_DOC_FAILURE = newProcessError("parse doc failure")

	//
	PROCESS_ERR_ITEM_TYPE_INVALID = newProcessError("pipe item type is invalid")

	// process panic invoke by outside of cobweb
	PROCESS_ERR_UNKNOWN = newProcessError("panic outside from cobweb")

	ERR_CONCURRENT_REQUEST_LIMIT = errors.New("concurrent request limit")
	ERR_BANNED_HOST              = errors.New("banned host")
	ERR_REQUEST_INTERVAL_LIMIT   = errors.New("request time interval")
)
