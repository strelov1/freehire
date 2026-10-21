// Package directapply is a candidate's application to an employer vacancy that takes
// applications on freehire (job.IsDirectApply): checking it against the employer's
// questions, recording it once, and delivering it to the employer's confirmed work email.
// See openspec/changes/add-in-app-apply.
package directapply

import "errors"

// ErrInvalid wraps every refusal of an application's content. Its text after the colon is
// user-facing: the handler renders it in the 400 body.
var ErrInvalid = errors.New("invalid application")
