# MDP Racing Line Mapper
A Markov Decision Process (MDP) based system for mapping out the optimal racing line on race tracks with configurable physics parameters (such as car's weight, horsepower, top speed, and track grip level, etc.)

Currently a work in progress, ~~but you can see a very basic implementation of the system - the car just runs in one big oval, which is boring and uninspired (\*cough\* *NASCAR* \*cough\*).~~ and while you can see screenshots of the current state, it's not very impressive just yet because the agent can't finish a single lap on either Monza or Spa. We'll get there soon enough, though.

## Running

```bash
$ go run cmd/app/main.go
```

## Prerequisites

- Go 1.22.4
- Associated Go libs (can be installed by running ```go mod download``` on this repository folder of course)

## Current State

![Current State 1](screenshots/current_state_2026_01_12.jpg)
![Current State 2](screenshots/current_state_2026_01_12_2.jpg)

The visualization now features:
- **Dark grayscale aesthetic**: Dark gray tarmac (80,80,80) on near-black background (10,10,10) for reduced eye strain
- **Frenet frame mesh overlay**: Green ribs showing the track centerline mesh used for agent state discretization
- **Dynamic HUD**: Status monitor (top-left) and agent parameters (top-right) that scale with window size
- **Path visualization**: Current lap (yellow), best lap (light green), and lap history (fading magenta trails)
- **Direction markers**: Red start line and yellow direction indicator for explicit initial heading

### Configuration

All simulation parameters are now consolidated at the top of `cmd/app/main.go` for easy tweaking:
- **Input track path**: Switch between different preprocessed tracks
- **Window dimensions**: Adjust render window size
- **Simulation settings**: Training speed multiplier, car spawn position, view margins
- **Color scheme**: All surface and visualization colors in one place

### Mesh Generation

The track mesh generation has been significantly refined:
- **Yellow dot direction markers**: Manually place a yellow dot on the input image to explicitly define the initial track direction, eliminating the need for algorithmic guessing
- **Optimized resolution**: `stepSize = 6.0` provides a balance between curve accuracy and performance
- **Multi-pass refinement**: 
  1. Initial pathfinding with visited-cell tracking and turning penalties
  2. "Elastic Band" centering pass (10 iterations) to pull waypoints toward true centerline
  3. Position smoothing (window=3) to remove jitter while preserving corner geometry
  4. Separate normal smoothing (window=5) to eliminate visual "spikes" in Frenet frames
- **Adaptive track width detection**: Automatically measures track width at start position for accurate mesh generation

## Current State

**This is still very much a work in progress.** The car doesn't even complete a single lap on real tracks like Monza yet. The agent crashes constantly, exploration is chaotic, and the Q-learning implementation needs serious work before it can produce anything resembling an optimal racing line.

Current blockers:
- **Agent can't complete laps**: The car crashes within seconds on complex tracks
- **Epsilon decay too aggressive**: Exploration drops off before the agent learns basic track navigation
- **Reward function needs tuning**: Current rewards don't effectively guide the agent toward lap completion
- **State space might be too granular**: The car is making micro-adjustments every tick, leading to sinusoidal behavior on straights
- **Mesh fitting in hairpins**: While improved, the Frenet frames still don't perfectly capture tight corners without increasing resolution

The visualization and preprocessing pipeline are solid, but the actual learning component—the whole point of this project—is nowhere near functional. I'm documenting this honestly because pretending otherwise would be pointless.

Next steps involve either:
1. Implementing staged training (learn to stay on track first, optimize line later)
2. Switching to Deep Q-Networks for better state representation
3. Completely rethinking the reward structure
4. Adding steering penalties to discourage the sinusoidal behaviour on relative straights

Or maybe all of the above. We'll see.

## Technical details for nerds

Currently the agent uses a Q-Table for learning, but I plan on implementing Deep Q in the future.

### State space

The state space is defined by the car's position, velocity, and heading. The car's position is discretized into a grid of cells, and the agent can take one of four actions at each cell: go straight, go left, go right, or go back (reverse).

### Current limitations
- Physics engine/logic - the physics characteristics are entirely vibe-coded with AI's help - I have only briefly skimmed the surface myself, and I might review it more extensively in the future. But immediately, I only plan on tweaking the units so that it matches real world speeds/acceleration/braking pressure/laptimes etc. (And if time permits, maybe grip/slip angles and the rest of handling-associated physics characteristics too). I'm naturally open to critical review and suggestions here - in fact I welcome it.
- The track layouts aren't 100% accurate - some very fine details are lost during the image processing stage. But it's still, like, 98-99% accurate.
- Most tracks have inconsistent widths - typically ranging from around 7m to around 14-16m - they get wide near the pit lanes and/or at the starting locations. But this system does not account for that unfortunately. I have designed this to only take the average track width in meters of each of these real world tracks and apply it to the track mesh - the resulting track mesh essentially has uniform track widths. This means wider/narrower corners, straights, etc., which could significantly change braking zones, steering angles, apexes, and ultimately lap times. So I don't expect these things to match the ideal telemetry or lap times of these tracks in the real world (although it'll be interesting to see how close we can get).

## Tracks

Some preliminary input tracks are stored in the `input_track_maps` directory. 

### Image processing
To transform the input images into the format expected by the system, some morphological image processing operations are performed on the inputs found in `input_track_maps/`, namely:
- Manual cropping
I had to manually crop these images by hand so that only the track layout can be fed to our script - this removes the shutterstock footer and the title of the track and other details. Unfortunately I can't think of a way to automate this process, but I'm (almost always) open to ideas/suggestions.
- Thresholding
To produce a black and white image of the track map - this stage acts as a filter to remove all the unnecessary visual elements from our source images from shutterstock - such as the numbers, text, etc. All we want is the track's layout.
- Morphological Opening 
Since opening is the combination of erosion and dilation, this stage achieves 2 things: remove the thin pit lanes and small objects to smooth out the edges of the track, and also unconnected white pixel "leftovers" from the digits of corner/turn numbers in the source images, all while maintaining the original track's width.

I spent way too much time trying to automate the "ideal" kernel size for this stage. Different tracks have different source resolutions, so a kernel that cleans Monza (12px wide) perfectly will completely dissolve Spa (6px wide). I eventually settled on a dynamic calculation: the script "probes" the track to find its input width, then sets the kernel size to **`Floor(inputWidth * 1.1)`** (clamped to at least 3). This seems to be the sweet spot for these specific shutterstock maps.

- Resolution Calibration & Rescaling
The system now automatically rescales input images to match our simulation's world scale. It checks for a pattern in the filename (e.g., `monza_10m.jpg`), calculates the actual pixel-width of the track, and resizes the entire image so that the track width in pixels matches our target meters at the simulation's `PixelsPerMeter` scale.

- Manual Start Markers
I tried (and failed) to use macro-template matching to automatically find the checkered finish line. ORB features were too noisy, and template correlation was matching random track curves and watermarks. So I've given up on full automation there—for now, manual intervention is required. You have to open the input image and draw a few **green dots** (at least 4px in diameter) where the start line should be. The preprocessor picks these up and converts them into the red start strip used by the simulation.

- Skeletonization
Essentially, thinning the track loop down to a single pixel to make the next step practically solvable.

- Connecting endpoints (i.e., "linking" the broken contours)
For each of the broken contours (segments of the track), the endpoints are found, and each of these endpoints are connected to their respective nearest neighbouring endpoints from other contours by drawing a straight line. This can at times cause inaccuracy—especially if gaps were present in curved parts of the track, those curved gaps will be filled with straight lines, so if this happens on hairpins it will look weird and result in very far-from-reality behaviour. I could not come up with a better, more accurate working solution in a reasonable period of time. Perhaps I'll revisit this some day.

- Restoration of track width thickness is far from accurate—after skeletonization, the track is restored to its "target" thickness based on the real-world width and simulation scale. This means the whole track will have constant width, as opposed to varying widths of these tracks in real life. This isn't too big of an issue as I'm only training agents to map out the optimal racing line by hotlapping alone.

As of now, I am experimenting with (cropped) track images from [this artist's shutterstock page](https://www.shutterstock.com/g/jzsoldos).

And I am working on the `debug_preproc.go` script to experiment and hone in on the right sequence of preprocessing steps outlined above. Running this script requires its own separate prerequisites (not required to run the main program itself) so ensure you have the following installed if you wish to generate meshes for other tracks:
- OpenCV (standalone C++ library, not your average pip installation of cv2),
- VTK (Visual Toolkit, for meshing the track),
- HDF5 (for saving the track mesh),

## Physics

The simulation uses a custom "Arcade" physics model that balances simplicity with the necessary dynamics for racing line optimization.

### Scale & Dimensions
- **World Scale**: 1 meter = 2.0 pixels (`PixelsPerMeter` in `internal/common/constants.go`).
- **Car Model**: The car is modeled as a rectangle with realistic dimensions:
    - **Width**: ~2.0 meters (4.0 pixels).
    - **Length**: ~4.5 meters (9.0 pixels).

### Dynamics
- **Inertia & Grip**: The car's velocity vector doesn't immediately snap to its heading. Instead, it "lerps" (linearly interpolates) towards the target heading based on a **Grip Factor**.
    - **Tarmac**: High grip (0.9), allowing for sharp, precise turns.
    - **Gravel/Off-track**: Low grip (0.5), causing the car to slide and lose directional control.
- **Movement Forces**:
    - **Acceleration/Braking**: Direct scalar adjustments to speed.
    - **Friction**: A constant decay factor simulating air resistance and rolling resistance.
    - **Terrain Resistance**: Driving on gravel applies a significantly higher friction penalty.

### Collision Detection
- **4-Corner Precision**: Collision is not checked at a single point. Instead, the system calculates the world-space coordinates of all **four corners** of the rectangular chassis every tick.
- **Crash Mechanics**: If any corner of the car touches a `CellWall` (typically the white space in track images), the car is marked as `Crashed`, speed is zeroed, and the agent receives a major penalty.
