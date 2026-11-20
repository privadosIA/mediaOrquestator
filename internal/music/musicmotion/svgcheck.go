package musicmotion

// SVGProblem says what is wrong with a drawing (a script, a foreignObject, an event handler, something loaded from outside), or "".
// It is the check the theme's assets and the characters go through, exported so that the pictures a HyperFrames block brings
// are judged by the same rule.
func SVGProblem(svg string) string { return svgProblem(svg) }
