package importer

type ProgressEvent struct {
	Stage   string
	Current int
	Total   int
	Message string
	Done    bool
}

type ProgressFunc func(ProgressEvent)

func emitProgress(fn ProgressFunc, ev ProgressEvent) {
	if fn != nil {
		fn(ev)
	}
}
