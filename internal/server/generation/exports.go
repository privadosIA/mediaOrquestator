package generation

// exports.go: the types of the answers and requests of the family under exported names, for the tests of the server
// that check them against openapi.yaml.

type (
	// GenerationRequest is the body of POST /api/generations (the `request` part when there is a picture).
	GenerationRequest = generationRequest
	// GenerationView is a generation as the API shows it.
	GenerationView = generationView
	// GenerationInput is a file a generation used, with the read-only address it can be fetched from.
	GenerationInput = generationInput
	// GenerationInputs is the `inputs` object of an option. A reference kind that is not there is not accepted; the two
	// pictures are always said.
	GenerationInputs = generationInputs
	// GenerationOptions is the answer of GET /api/generation/options.
	GenerationOptions = generationOptions
	// DurationRange is the shortest, the longest and the default duration in seconds of a clip an option makes.
	DurationRange = durationRange
	// InputLimit is how many files of a kind an option takes and, for videos and audios, how long each may be.
	InputLimit = inputLimit
	// VideoOptionView is one way of making a video, as the options list shows it.
	VideoOptionView = videoOptionView
	// ImageModelView is one model a picture can be edited with.
	ImageModelView = imageModelView
	// ImageEditRequest is the `request` part of POST /api/image-edits.
	ImageEditRequest = imageEditRequest
	// ImageEditResult is one picture of an edit: the seed that makes it again and where to fetch it.
	ImageEditResult = imageEditResult
	// ImageEditView is an edit as the API shows it.
	ImageEditView = imageEditView
	// VideoEditRequest is the `request` part of POST /api/video-edits.
	VideoEditRequest = videoEditRequest
	// VideoEditView is an edit as the API shows it.
	VideoEditView = videoEditView
	// VideoEditResult is what a finished edit made, as the API shows it.
	VideoEditResult = videoEditResult
	// VideoEditToolView is a tool of GET /api/video-edits/options.
	VideoEditToolView = videoEditToolView
	// LibraryMotionView is a motion as the API shows it.
	LibraryMotionView = libraryMotionView
	// LibraryMotionRequest is the `request` part of POST /api/motions.
	LibraryMotionRequest = libraryMotionRequest
	// PresetRequest is the body that asks for a style to be drawn from a reference of the library.
	PresetRequest = presetRequest
	// PresetPut is the body that saves a style (its design, rhythm and parts) under a name.
	PresetPut = presetPut
)
