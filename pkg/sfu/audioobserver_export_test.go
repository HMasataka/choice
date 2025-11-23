package sfu

func (a *AudioObserver) ExportSortStreamsByActivity(streams []*AudioStream) []*AudioStream {
	return a.sortStreamsByActivity(streams)
}
