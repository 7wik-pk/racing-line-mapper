# MDP Racing Line Mapper
A Markov Decision Process (MDP) based system for mapping out the optimal racing line on race tracks with configurable physics parameters (such as car's weight, horsepower, top speed, and track grip level, etc.)

Currently a work in progress, but you can see a very basic implementation of the system - the car just runs in one big oval, which is boring and uninspired (\*cough\* *NASCAR* \*cough\*).

## Running

```bash
$ go run cmd/app/main.go
```

## Prerequisites

- Go 1.22.4
- Associated Go libs (can be installed by running ```go mod download``` on this repository folder of course)

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
Since opening is the combination of erosion and dilation, this stage achieves 2 things: remove the thin pit lanes and small objects to smooth out the edges of the track, and also unconnected white pixel "leftovers" from the digits of corner/turn numbers in the source images, all while maintaining the original track's width - thanks to . This produces broken contours due to watermarks and other artefacts in the source/input images.

Unfortunately this stage also requires manual intervention - different tracks require different kernel sizes for opening to achieve the desired result. It'll be a very daunting task to automate this, so I've moved on to other things instead for now.

- Skeletonization
Essentially, thinning the track loop down to a single pixel to make the next step practically solvable.
- Connecting endpoints (i.e., "linking" the broken contours)
For each of the broken contours (segments of the track), the endpoints are found, and each of these endpoints are connected to their respective nearest neighbouring endpoints from other contours by drawing a straight line. This can at times cause inaccuracy - especially if gaps were present in curved parts of the track, those curved gaps will be filled with straight lines, so if this happens on hairpins it will look weird and result in very far-from-reality behaviour. I could not come up with a better, more accurate working solution in a reasonable period of time. Perhaps I'll revisit this some day.
- Restoration of track width thickness is far from accurate - after skeletonization, the track is simply dilated to the thickness of the original image's thickness level - which essentially means the whole track will have constant width, as opposed to varying widths of these tracks in real life. This isn't too big of an issue as I'm only training agents to map out the optimal racing line by hotlapping alone.

As of now, I am experimenting with (cropped) track images from [this artist's shutterstock page](https://www.shutterstock.com/g/jzsoldos).

And I am working on the `debug_preproc.go` script to experiment and hone in on the right sequence of preprocessing steps outlined above. Running this script requires its own separate prerequisites (not required to run the main program itself) so ensure you have the following installed if you wish to generate meshes for other tracks:
- OpenCV (standalone C++ library, not your average pip installation of cv2),
- VTK (Visual Toolkit, for meshing the track),
- HDF5 (for saving the track mesh),

