package config

import "fmt"

// Favorite marks one song of a music project as a keeper: the track and the
// seed it was made with. The seed, not the variant number, identifies the song
// so the mark survives changing how many variants a track makes (the same seed
// gives the same song).
type Favorite struct {
	Track string `yaml:"track" json:"track"`
	Seed  int64  `yaml:"seed" json:"seed"`
}

// validateFavorites checks the favorites of a music project. A favorite whose
// track is gone only warns: it is a mark, and the track may come back.
func (c *Config) validateFavorites() []error {
	var errs []error
	tracks := make(map[string]bool, len(c.Tracks))
	for _, t := range c.Tracks {
		tracks[t.ID] = true
	}
	seen := make(map[Favorite]bool, len(c.Favorites))
	for i, f := range c.Favorites {
		switch {
		case f.Track == "":
			errs = append(errs, fmt.Errorf("config: favorite at index %d has no track", i))
		case seen[f]:
			c.warn("favorite %s (seed %d) is listed more than once", f.Track, f.Seed)
		case !tracks[f.Track]:
			c.warn("favorite %q (seed %d) points to a track that is not in the project", f.Track, f.Seed)
		}
		seen[f] = true
	}
	return errs
}
