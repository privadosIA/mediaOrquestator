package settings

import (
	"errors"
	"fmt"
)

// The tasks that can be routed to a provider.
const (
	// TaskAssistant is the prompt assistant: picture, movement, song and videoclip-storyboard prompts.
	TaskAssistant = "assistant"
	// TaskDescribeImage is describing a picture.
	TaskDescribeImage = "describe_image"
	// TaskDescribeVideo is describing a clip, as one scene or one per shot.
	TaskDescribeVideo = "describe_video"
	// TaskQuality is the vision judgement of the clip quality check.
	TaskQuality = "quality"
	// TaskMotionGraphics is writing the HTML page of a motion-graphics scene or overlay (a text model).
	TaskMotionGraphics = "motion_graphics"
)

// Tasks lists every routable task, in the order the screen shows them.
var Tasks = []string{TaskAssistant, TaskDescribeImage, TaskDescribeVideo, TaskQuality, TaskMotionGraphics}

// DefaultMaxCalls is the most calls one job may make to a remote provider unless the user changes it.
const DefaultMaxCalls = 60

// ValidTask reports whether name is a routable task.
func ValidTask(name string) bool {
	for _, t := range Tasks {
		if t == name {
			return true
		}
	}
	return false
}

// NeedsVision says whether a task uses the provider's vision model (the assistant and the motion-graphics
// writer use its text model).
func NeedsVision(task string) bool { return task != TaskAssistant && task != TaskMotionGraphics }

// ModelFor is the model a provider declares for a task: its text model for the
// assistant and the motion-graphics writer, its vision model otherwise.
func ModelFor(p Provider, task string) string {
	if NeedsVision(task) {
		return p.VisionModel
	}
	return p.TextModel
}

// RoutedProvider is the id of the provider a task is routed to (the built-in Ollama by default).
func RoutedProvider(task string) string {
	if id := Load().Routing[task]; id != "" {
		return id
	}
	return BuiltinID
}

// SetRouting saves which provider runs each of the given tasks. Routing a task
// to the built-in provider removes its entry. The caller has validated the ids.
func SetRouting(changes map[string]string) error {
	mu.Lock()
	defer mu.Unlock()
	f, rerr := ReadFile() // a source that cannot be read is not "no settings": saving would erase them
	if rerr != nil {
		return rerr
	}
	if f.Routing == nil {
		f.Routing = map[string]string{}
	}
	for task, id := range changes {
		if id == "" || id == BuiltinID {
			delete(f.Routing, task)
		} else {
			f.Routing[task] = id
		}
	}
	if len(f.Routing) == 0 {
		f.Routing = nil
	}
	return Save(f)
}

// Acknowledge records that the user accepted sending data to a provider at its current address.
func Acknowledge(id string) error {
	mu.Lock()
	defer mu.Unlock()
	f, rerr := ReadFile() // a source that cannot be read is not "no settings": saving would erase them
	if rerr != nil {
		return rerr
	}
	var base string
	for _, p := range f.Providers {
		if p.ID == id {
			base = p.BaseURL
		}
	}
	if base == "" {
		return ErrNoSuchProvider
	}
	if f.Consent == nil {
		f.Consent = map[string]string{}
	}
	f.Consent[id] = base
	return Save(f)
}

// Acknowledged says whether the user accepted sending data to the provider at
// this address: editing the address makes it stop counting.
func Acknowledged(id, baseURL string) bool {
	return baseURL != "" && Load().Consent[id] == baseURL
}

// MaxCalls is the most calls one job may make to a remote provider.
func MaxCalls() int {
	if r := Load().Remote; r != nil && r.MaxCalls > 0 {
		return r.MaxCalls
	}
	return DefaultMaxCalls
}

// SetMaxCalls saves the cap; it must be at least 1.
func SetMaxCalls(n int) error {
	if n < 1 {
		return errors.New("the call cap must be at least 1")
	}
	mu.Lock()
	defer mu.Unlock()
	f, rerr := ReadFile() // a source that cannot be read is not "no settings": saving would erase them
	if rerr != nil {
		return rerr
	}
	f.Remote = &RemoteSettings{MaxCalls: n}
	if n == DefaultMaxCalls {
		f.Remote = nil
	}
	return Save(f)
}

// ProviderInUse names the first task routed to the provider ("" when none uses it).
func ProviderInUse(id string) string {
	routing := Load().Routing
	for _, t := range Tasks {
		if routing[t] == id {
			return t
		}
	}
	return ""
}

// ErrProviderInUse means a task is routed to the provider.
type ErrProviderInUse struct{ Task string }

func (e ErrProviderInUse) Error() string {
	return fmt.Sprintf("the task %q is routed to this provider: route it elsewhere first", e.Task)
}
