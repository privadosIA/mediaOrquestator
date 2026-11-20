package llm

import "context"

// Preparer rewrites a request just before it is sent (for example to shrink its prompt). It must
// never fail the call: when it cannot improve the request it returns it unchanged.
type Preparer interface {
	Prepare(ctx context.Context, req Request) Request
}

// prepare is the installed Preparer, or nil. Set once at start-up with SetPreparer.
var prepare Preparer

// SetPreparer installs the hook every provider applies to its requests; nil removes it.
func SetPreparer(p Preparer) { prepare = p }

func prepared(ctx context.Context, req Request) Request {
	if prepare == nil {
		return req
	}
	return prepare.Prepare(ctx, req)
}
