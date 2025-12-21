# DQN Migration Roadmap

This document outlines the architectural changes required to upgrade the Racing Line Mapper from a Tabular Q-Learning agent to a Deep Q-Network (DQN) agent.

## 1. State Representation (Discrete -> Continuous)
**Goal:** Remove the information loss caused by `DiscretizeState`.

*   **Current (Q-Table)**:
    *   `State` is a struct of integers (Segment, Lane, SpeedLevel).
    *   Requires mapping continuous physics to coarse "buckets".
*   **New (DQN)**:
    *   `State` becomes a `[]float64` vector (Tensor).
    *   **Inputs**:
        1.  `Normalized Lateral Offset` (-1.0 to 1.0, where 0 is center).
        2.  `Normalized Speed` (Car.Speed / MaxSpeed).
        3.  `Heading Error` (Difference between Car Heading and Track Tangent, sin/cos components).
        4.  `Track Curvature` (Lookahead: change in tangent for next few waypoints).
        5.  `Lidar/Raycasts` (Optional): Normalized distances to track edges at angles [-45, 0, 45].

## 2. The Brain: Neural Network
**Goal:** Replace the `map[State]Action` lookup with a Function Approximator.

*   **Architecture**:
    *   Input Layer: Size = `len(StateVector)` (approx 4-8 inputs).
    *   Hidden Layer 1: 64 Neurons (ReLU activation).
    *   Hidden Layer 2: 64 Neurons (ReLU activation).
    *   Output Layer: Size = `ActionCount` (5). Linear activation (Raw Q-Values).

*   **Implementation Options in Go**:
    1.  **Gorgonia**: Standard Go ML library (Graph-based, similar to Theano/TensorFlow).
    2.  **Custom MLP**: Write a simple standard Feed-Forward network from scratch.
        *   Need basic `Matrix * Vector` operations.
        *   Need `Backpropagation` (Gradient Descent) implementation.
        *   *Pros*: No heavy Cgo dependencies, very fast for small nets, educational.
        *   *Cons*: Manual implementation of optimizers (Adam/SGD).

## 3. Training Loop Changes
**Goal:** Stabilize training using Experience Replay and Target Networks.

### A. Experience Replay
Neural networks fail if trained on highly correlated sequential data (e.g., frame 1, frame 2, frame 3...).
*   **New Struct**: `ReplayBuffer`
    *   Fixed size circular array (e.g., 10,000 items).
    *   Stores `Transition {State, Action, Reward, NextState, Done}`.
*   **Loop Update**:
    *   `Agent.Step()`: Do **NOT** train immediately. Just push transition to Buffer.
    *   `Agent.Train()`: Sample random batch (e.g., 32 items) from Buffer. Calculate Loss. Update Weights.

### B. Target Switch
*   Use two networks: `PolicyNet` (updates every step) and `TargetNet` (updates every N steps or via "Soft Update").
*   Q-Target calculation uses `TargetNet` to estimate future value, preventing feedback loops where values spiral out of control.

## 4. Hyperparameters
DQN introduces new parameters to tune:
*   **Batch Size**: 32 or 64.
*   **Hidden Size**: 32 to 128.
*   **Update Frequency**: How often to train (e.g., every 4 steps).
*   **Target Update Rate (Tau)**: For soft updates (e.g., 0.001).

## 5. Migration Steps
1.  **Refactor Agent Interface**: Ensure `SelectAction` accepts `[]float64`.
2.  **Implement Replay Buffer**: Create a concurrent-safe ring buffer.
3.  **Implement Network**: Choose library or write simple Matrix math struct.
4.  **Connect**: Hook up the batch sampling loop in `main.go`.

# Manual notes for future scope / other things to consider

Consider adding staged training - each stage to teach the DQN different things with different rewards structures - such as first stage could be in an oval/rectangular circuit where we teach the agent going outside the track is bad, another stage where being fast out of corners is good, etc.
Consider making the DQN weights storeable - so that a ready-made agent model can be stored after the first few staged training stages to be used on actual track images
Finally, consider making the agent universal - i.e., a DQN that is trained on all tracks and can "observe" the whole track. What track the agent is in (or the track's encoding in some way) should be extra dimensions of state space (or input features fed into the network).