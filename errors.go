package intervalsicu

import "errors"

// asError is errors.As specialised to *Error, kept separate so the exported
// helpers above read as one line each.
func asError(err error, target **Error) bool {
	return errors.As(err, target)
}
