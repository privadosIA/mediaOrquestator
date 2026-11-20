package videoedit

import (
	"context"
	"os/exec"
	"sync"
)

// Catalog is the code behind the tools and everything that code needs from the machine: the programs it runs, where
// the networks are, the services the server installs. A server, the command and every test make their own, so no
// state is shared between them. The exported function fields are the ones a test replaces (they are set once, before
// the Catalog is used); NewCatalog fills them with the real thing.
type Catalog struct {
	// Probe reads a video (ffprobe).
	Probe ProbeFunc
	// ToolsDir is the folder that holds bin/realesrgan and bin/rife.
	ToolsDir func() string
	// RunProgram runs a program of the networks and gives its output.
	RunProgram func(ctx context.Context, name string, args []string) ([]byte, error)
	// FreeBytes is the room there is on the disk of a folder.
	FreeBytes func(dir string) uint64
	// RunSD runs stable-diffusion.cpp in a folder with extra environment.
	RunSD func(ctx context.Context, dir string, env, args []string) ([]byte, error)
	// LookPath finds a program.
	LookPath func(name string) (string, error)
	// GPUMemoryMB is the memory of the biggest graphics card.
	GPUMemoryMB func() (int, error)

	mu        sync.RWMutex
	impls     map[string]Impl
	vision    Vision
	clipMaker ClipMaker
	library   Library
}

// NewBareCatalog is a Catalog with the real programs and no tool: the tests register stubs in it.
func NewBareCatalog() *Catalog {
	return &Catalog{
		Probe: ffprobe, ToolsDir: defaultToolsDir, RunProgram: runProgram, FreeBytes: freeBytes,
		RunSD: runSD, LookPath: exec.LookPath, GPUMemoryMB: gpuMemoryMB,
		impls: map[string]Impl{},
	}
}

// NewCatalog is a Catalog with every tool of this build registered, bound to it.
func NewCatalog() *Catalog {
	c := NewBareCatalog()
	for id, run := range map[string]func(context.Context, Job) (Result, error){
		Scale: c.runScale, Interpolate: c.runInterpolate, Reframe: c.runReframe,
	} {
		c.Register(id, Impl{Run: run, Available: ffmpegAvailable})
	}
	c.Register(Extend, Impl{Run: c.runExtend, Available: c.extendAvailable})
	c.Register(MotionControl, Impl{Run: c.runMotionControl, Available: c.motionAvailable})
	c.Register(RemoveBackground, Impl{Run: c.runRemoveBackground, Available: c.availableWithVisionAndFFmpeg})
	c.Register(Depth, Impl{Run: c.runDepth, Available: c.availableWithVisionAndFFmpeg})
	return c
}

// Register installs (or, with a nil Run, removes) the code of a tool.
func (c *Catalog) Register(id string, impl Impl) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if impl.Run == nil {
		delete(c.impls, id)
		return
	}
	c.impls[id] = impl
}

// ImplOf is the code of a tool, when it has any.
func (c *Catalog) ImplOf(id string) (Impl, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	i, ok := c.impls[id]
	return i, ok
}

// IDsWithCode lists the tools that have code, in no particular order.
func (c *Catalog) IDsWithCode() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.impls))
	for id := range c.impls {
		out = append(out, id)
	}
	return out
}

// AvailabilityOf says whether a tool can run now, with the reason when it cannot.
func (c *Catalog) AvailabilityOf(id string) (bool, string) {
	i, ok := c.ImplOf(id)
	if !ok {
		return false, "this build does not have the tool yet"
	}
	if i.Available != nil {
		return i.Available()
	}
	return true, ""
}

// SetVision installs the vision service (nil removes it).
func (c *Catalog) SetVision(v Vision) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.vision = v
}

// VisionOf is the installed vision service, or nil.
func (c *Catalog) VisionOf() Vision {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.vision
}

// SetClipMaker installs the maker of clips (nil removes it).
func (c *Catalog) SetClipMaker(m ClipMaker) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clipMaker = m
}

// ClipMakerOf is the installed maker, or nil.
func (c *Catalog) ClipMakerOf() ClipMaker {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clipMaker
}

// SetLibrary installs the library of motions (nil removes it).
func (c *Catalog) SetLibrary(l Library) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.library = l
}

// LibraryOf is the installed library, or nil.
func (c *Catalog) LibraryOf() Library {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.library
}
