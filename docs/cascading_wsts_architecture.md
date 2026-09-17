# Cascading Weighted State Transition Systems (WSTS) for Entity Behavior

## 1. Architectural Overview

In complex emergent simulations (such as *RimWorld*, *Dwarf Fortress*, or *The Sims*), managing agent decision-making via monolithic state machines or rigid behavior trees leads to combinatorial explosion, fragile prioritization logic, and unnatural, mechanical reactions.

The **Cascading Weighted State Transition System (WSTS)** architecture decomposes agent cognition into discrete, interacting layers of state machines. Instead of treating internal drives (such as physiological needs or psychological mood) as passive scalar bags read by a single decision tree, **Need** and **Mood** operate as autonomous, continuous/discrete WSTSs that emit state-dependent bias vectors and urgency profiles into an executive **Action WSTS**.

```
                       [ External / Environmental Triggers ]
                          (SawMonster, AteMeal, Hypothermia)
                                    │           │
                     ┌──────────────┘           └──────────────┐
                     ▼                                         ▼
         ┌───────────────────────┐                 ┌───────────────────────┐
         │       Need WSTS       │                 │       Mood WSTS       │
         │ (Physiological Phase) │                 │ (Psychological Phase) │
         │   • Satiated          │                 │   • Content           │
         │   • Peckish           │                 │   • Panicked          │
         │   • Starving          │                 │   • Depressed Spiral  │
         └───────────┬───────────┘                 └───────────┬───────────┘
                     │                                         │
                     │ Urgency Profile                         │ Behavioral Bias
                     │ Vector: U_need                          │ Matrix: M_bias
                     └───────────────────┬─────────────────────┘
                                         ▼
                             ┌───────────────────────┐
                             │      Action WSTS      │
                             │  (Executive Arbiter)  │
                             │   • Idle              │
                             │   • Eat               │
                             │   • Flee              │
                             │   • Work / Haul       │
                             └───────────────────────┘
```

---

## 2. Theoretical Formulation

A Weighted State Transition System is formally defined as a tuple:

$$\mathcal{T} = \left(S, \Sigma, \delta, \mathbb{K}, w, s_0\right)$$

Where:
* $S$ is the discrete set of states.
* $\Sigma$ is the alphabet of input events/stimuli.
* $\delta \subseteq S \times \Sigma \times S$ is the transition relation.
* $\mathbb{K} = (K, \oplus, \otimes, \bar{0}, \bar{1})$ is a semiring defining weight composition.
* $w: \delta \to K$ is the dynamic weight function assigning a semiring value to each edge.
* $s_0 \in S$ is the initial state.

In this cascading simulation architecture, the layers are evaluated over an **Argmax / Optimization Semiring** $(\mathbb{R} \cup \{-\infty\}, \max, +, -\infty, 0)$:

### 2.1 The Need WSTS (Physiological Phase Machine)
* **Internal Coordinates:** $\vec{c}_N \in [0, 1]^k$ (continuous tracking of calories, hydration, rest, body temperature).
* **Discrete States ($S_N$):** `Satiated`, `Peckish`, `Ravenous`, `Starving`, `Moribund`.
* **Dynamics:**
  Transitions between physiological states do not trigger instantly on boundary crossings. Edge weights factor in metabolic consumption rate and recovery latency, providing natural hysteresis.
* **Output / Emission:**
  Emits an **Urgency Profile Vector**:
  $$\vec{U}_{\text{need}} \in \mathbb{R}_{\ge 0}^p$$
  where each component corresponds to an actionable physiological goal (e.g., $U_{\text{eat}}, U_{\text{sleep}}, U_{\text{warmth}}$).

### 2.2 The Mood WSTS (Psychological Phase Machine)
* **Discrete States ($S_M$):** `Content`, `Stressed`, `Panicked`, `Depressive Spiral`, `Berserk Primed`, `Shaken`.
* **Dynamics:**
  Governed by perceptual triggers ($e \in \Sigma_M$, e.g., `SawMonster`, `CorpseSeen`, `Insulted`, `ComfortableRoom`). Personality traits act as static multipliers on transition edge weights.
* **Output / Emission:**
  Emits a **Behavioral Bias Tensor / Matrix**:
  $$\mathbf{M}_{\text{bias}} \in \mathbb{R}^{p \times p}$$
  This matrix scales, dampens, or completely masks candidate action utilities (e.g., panic suppresses hunger urgency and inflates escape utility).

### 2.3 The Action WSTS (Executive Arbiter)
* **Discrete States ($S_A$):** Concrete motor activities (`Idle`, `Eat`, `Flee`, `Haul`, `Sleep`, `Fight`).
* **Candidate Transitions:** Feasible actions available in the immediate spatial/world context.
* **Composite Edge Evaluation:**
  When transitioning from the current state $s \in S_A$ to candidate state $s' \in S_A$, the dynamic weight is formulated as:

  $$W(s \to s') = \Big( \mathbf{M}_{\text{bias}}(S_M) \times \vec{U}_{\text{need}}(S_N) \Big) \cdot \vec{\phi}(s') + H(s, s') - \lambda \cdot \text{Cost}_{\text{spatial}}(s')$$

  Where:
  * $\vec{\phi}(s')$ is the intrinsic base payoff or utility of action $s'$.
  * $H(s, s')$ is an **inertia/hysteresis bonus** applied if $s' = s$, preventing high-frequency thrashing between near-equal priorities.
  * $\text{Cost}_{\text{spatial}}(s')$ is the pathfinding or execution distance penalty.
  * $\lambda$ is the agent's spatial resistance coefficient.

---

## 3. Concrete Scenario: Step-by-Step Execution Trace

To illustrate the cascading mechanics, consider an agent navigating a settlement when encountering a lethal threat.

### Baseline Conditions (Tick 100)
* **Need WSTS:** Current state is `Ravenous`. 
  * Hunger level is critically low.
  * Emits urgency: $\vec{U}_{\text{need}}(\text{Eat}) = 0.95$.
* **Mood WSTS:** Current state is `Content`.
  * Multipliers are standard: $\mathbf{M}_{\text{bias}}(\text{Eat}) = 1.0$, $\mathbf{M}_{\text{bias}}(\text{Flee}) = 1.0$.
* **Action WSTS:** Currently `WalkingToPantry` with intent to `Eat`.
  * $W(\text{Eat}) = 1.0 \times 0.95 \times 100 - \text{PathCost}(12) = 83$.
  * $W(\text{Flee}) = 0$.

---

### Step 1: Trigger Event `SawMonster` (Tick 101)
* **Event Dispatch:** The perception component emits event `SawMonster` with threat severity 0.85.
* **Need WSTS Reaction:**
  * Physiological states do not process sensory threat symbols.
  * State remains `Ravenous`; metabolic consumption continues uninterrupted.
* **Mood WSTS Evaluation:**
  * Evaluates outgoing transitions from `Content`:
    * Edge $w(\text{Content} \xrightarrow{\text{SawMonster}} \text{Panicked}) = 0.85 \times \text{TraitSensitivity}$.
  * The weight exceeds the panic threshold.
  * **Transition:** `Content` $\to$ `Panicked`.
* **Cascade Emission Update:**
  * Mood state `Panicked` immediately re-projects $\mathbf{M}_{\text{bias}}$:
    * Physiological urgency dampener: $\mathbf{M}_{\text{bias}}(\text{Eat}) \leftarrow 0.0$ (fight-or-flight sympathetic response inhibits digestion/appetite).
    * Threat mitigation multiplier: $\mathbf{M}_{\text{bias}}(\text{Flee}) \leftarrow 12.0$.

---

### Step 2: Action WSTS Preemption Arbitration (Tick 102)
* The Action WSTS receives the updated bias matrix during its interruption check:
  * $W(\text{Maintain: Eat}) = 0.0 \times 0.95 + H(\text{Eat}, \text{Eat}) = 10$.
  * $W(\text{Transition: Flee}) = 12.0 \times \text{ThreatProximityScore} = 250$.
* **Outcome:** The `Flee` transition dominates. Current task is preempted; the agent immediately drops pathing to the pantry and begins evasive locomotion.

---

### Step 3: Threat Neutralization `MonsterDied` (Tick 180)
* **Event Dispatch:** Colony defenses dispatch the monster; event `MonsterDied` fires.
* **Mood WSTS Inertia / Asymmetric Recovery:**
  * Crucially, psychological systems do not instantly revert to baseline.
  * The direct transition $\text{Panicked} \xrightarrow{\text{MonsterDied}} \text{Content}$ is illegal or weighted with an infinite cost penalty.
  * Instead, the machine transitions into an intermediate state:
    $$\text{Panicked} \xrightarrow{\text{MonsterDied}} \text{Shaken}$$
  * In `Shaken`, adrenaline decays continuously over time. The threat multiplier scales down from $12.0 \to 1.5$, while the inhibition on physiological drives partially lifts:
    $$\mathbf{M}_{\text{bias}}(\text{Eat}) \leftarrow 0.70$$
* **Action Resolution:**
  * Need state `Ravenous` has persisted and accumulated additional urgency ($\vec{U}_{\text{need}}(\text{Eat}) = 0.98$).
  * Re-evaluating candidate actions:
    * $W(\text{Flee}) = 1.5 \times 0.0 = 0$.
    * $W(\text{Eat}) = 0.70 \times 0.98 \times 100 - \text{PathCost}(15) = 53.6$.
    * $W(\text{Wander/Recover}) = 30.0$.
  * **Outcome:** The agent immediately resumes seeking food, but retains psychological debuffs associated with the `Shaken` state (e.g., increased clumsiness, elevated risk of mental breakdown).

---

## 4. Key Architectural Benefits

| Aspect | Pure FSM / Rigid Tree | Cascading WSTS |
| :--- | :--- | :--- |
| **State Explosion** | Requires composite states like `EatingWhilePanicked`. | State counts scale additively ($|S_N| + |S_M| + |S_A|$) rather than multiplicatively ($|S_N| \times |S_M| \times |S_A|$). |
| **Hysteresis & Thrashing** | Hardcoded cooldown timers and flag checks. | Built-in algebraic inertia terms ($H(s, s')$) and structural intermediate recovery states (`Shaken`). |
| **Personality Traits** | Deeply nested condition checks across behavior trees. | Static weight modifiers applied to edges in the Mood or Need WSTSs without altering executive action code. |
| **Emergent Pathology** | Must be explicitly scripted as separate behavior branches. | Emerges naturally from tensor coupling (e.g., chronic stress biasing need-satisfaction toward stress-eating or substance abuse). |
