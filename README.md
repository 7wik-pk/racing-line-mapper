# MDP Racing Line Mapper
A Markov Decision Process (MDP) based system for mapping out the optimal racing line on race tracks with configurable physics parameters (such as car's weight, horsepower, top speed, and track grip level, etc.)

Currently a work in progress, but you can see a very basic implementation of the system - the car just runs in one big oval, which is boring and uninspired (\*cough\* *NASCAR* \*cough\*).

## Running

```bash
$ go run cmd/app/main.go
```

## Prerequisites
- Go 1.22.4
- OpenCV 4.8.0
- Ebiten 2.1.1

## Tracks

Some preliminary input tracks are stored in the `input_track_maps` directory. Currently I'm trying to figure out the right sequence of image preprocessing steps to prepare the input images in the format expected by the system.