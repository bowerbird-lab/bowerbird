package plugin

// OfficialManifests returns the core native plugins maintained by the Bowerbird
// team. Only core media playback plugins (audio and video, alongside built-in
// image preview) are registered as built-ins; markdown, PSD, and AI prompt
// metadata are external plugins installed from bowerbird-lab/plugins.
func OfficialManifests() []Manifest {
	return []Manifest{
		{
			ID:           "official.audio-player",
			Name:         "Audio Player",
			Version:      "2.0.0",
			Kind:         "preview",
			Native:       "audio",
			Priority:     60,
			Description:  "Native audio playback with spectrum, scrubber and metadata.",
			Author:       "Bowerbird Team",
			License:      "MIT",
			Extensions:   []string{"mp3", "wav", "ogg", "flac", "m4a", "aac", "webm"},
			MimePrefixes: []string{"audio/"},
			Capabilities: []string{"preview"},
			Builtin:      true,
		},
		{
			ID:           "builtin.video",
			Name:         "Video Player",
			Version:      "2.0.0",
			Kind:         "preview",
			Native:       "video",
			Priority:     90,
			Description:  "Native video playback rendered by mygo/ui with ffmpeg frame decoding and mygo audio output.",
			Author:       "Bowerbird Team",
			License:      "MIT",
			Extensions:   []string{"mp4", "mov", "qt", "webm", "mkv", "avi", "m4v", "flv", "f4v", "mts", "m2ts", "m3u8", "m3u", "mpd", "ogv", "ogm", "3gp", "3g2", "wmv", "asf", "mpg", "mpeg", "m2v", "vob", "rmvb", "rm"},
			MimePrefixes: []string{"video/", "application/vnd.apple.mpegurl", "application/x-mpegurl", "application/dash+xml"},
			Capabilities: []string{"preview"},
			Builtin:      true,
		},
	}
}
