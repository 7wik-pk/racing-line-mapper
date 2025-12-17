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

### Tracks

Some preliminary input tracks are stored in the `input_track_maps` directory. Currently I'm figuring out the right sequence of image preprocessing steps (Canny edge detection, erosion & closing filters, etc.) to prepare the input images in the format expected by the system.

As of now, among others, I am experimenting with track images from [this artist's shutterstock page](https://www.shutterstock.com/g/jzsoldos).

And I am working on the `debug_preproc.go` script to experiment and hone in on the right sequence of preprocessing steps outlined above. Running this script requires its own separate prerequisites (not required to run the main program itself) so ensure you have the following installed if you wish to generate meshes for other tracks:
- OpenCV (standalone C++ library, not your average pip installation of cv2),
- VTK (Visual Toolkit, for meshing the track),
- HDF5 (for saving the track mesh),

