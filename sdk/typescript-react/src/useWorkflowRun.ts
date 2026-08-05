import { useCallback, useEffect, useRef, useState } from "react";
import type { PatchcordClient, Run, RunSnapshot, RunStep, RunWorkflowOptions } from "@glmtsolutions/patchcord-sdk";

/**
 * Where a useWorkflowRun hook's tracked run currently stands. "idle" is the
 * state before start() has ever been called, or after reset() — a UI
 * typically renders its input form in that phase and a progress/result view
 * otherwise.
 */
export type WorkflowRunPhase = "idle" | "running" | "succeeded" | "failed" | "cancelled";

export interface UseWorkflowRunResult {
  /** The hook's current lifecycle phase — see WorkflowRunPhase. */
  phase: WorkflowRunPhase;
  /** The live Run handle for the most recent start() call. Undefined in phase "idle". */
  run: Run | undefined;
  /** Merged step states, updated live as events arrive — RunSnapshot.steps from the underlying Run.watch(). */
  steps: RunStep[];
  /** The run's error, once phase is "failed" — or start() itself threw before the run was even created (e.g. a network error). */
  error: string | undefined;
  /** The run's outputs, once phase is "succeeded". */
  outputs: Record<string, unknown> | undefined;
  /** Starts a new run of workflowId, discarding any previous one's outcome. Never throws — failures surface through `error`/`phase` instead. */
  start: (options?: RunWorkflowOptions) => void;
  /** Cancels the in-flight run, if phase is "running". No-op otherwise. */
  cancel: () => void;
  /** Returns to phase "idle", discarding the last run's outcome — e.g. for a "run again" button that goes back to an input form. Does not cancel a still-running run; call cancel() first if that's the intent. */
  reset: () => void;
}

/**
 * Runs workflowId via `client` and keeps React state in sync with its live
 * progress over SSE (Run.watch()) — the hook form of the pattern this
 * package exists for: "run a workflow when the user clicks a button, show
 * its progress properly, with none of the plumbing hand-written per app".
 *
 * ```tsx
 * const { phase, steps, error, outputs, start } = useWorkflowRun(client, "hello_patchcord");
 *
 * <button onClick={() => start({ inputs: { text: "hi" } })} disabled={phase === "running"}>
 *   Run
 * </button>
 * ```
 */
export function useWorkflowRun(client: PatchcordClient, workflowId: string): UseWorkflowRunResult {
  const [phase, setPhase] = useState<WorkflowRunPhase>("idle");
  const [run, setRun] = useState<Run>();
  const [steps, setSteps] = useState<RunStep[]>([]);
  const [error, setError] = useState<string>();
  const [outputs, setOutputs] = useState<Record<string, unknown>>();

  // Two problems, one counter: (1) start() called again before a previous
  // run's watch loop finished must not let that stale loop go on
  // overwriting state meant for the new run; (2) the component unmounting
  // must not let an in-flight loop touch state at all. Every async step
  // below re-checks its own captured generation against the current one
  // before writing anything.
  const generationRef = useRef(0);
  const controllerRef = useRef<AbortController>();
  const runRef = useRef<Run>();

  useEffect(() => {
    return () => {
      generationRef.current += 1; // invalidates any in-flight loop for this instance
      // Only detaches from the run's event stream — the run itself keeps
      // executing on the agent. Call cancel() explicitly to actually stop
      // it; unmounting a component isn't reason enough to do that on the
      // user's behalf.
      controllerRef.current?.abort();
    };
  }, []);

  const start = useCallback(
    (options?: RunWorkflowOptions) => {
      const generation = (generationRef.current += 1);
      controllerRef.current?.abort();
      const controller = new AbortController();
      controllerRef.current = controller;

      setPhase("running");
      setRun(undefined);
      setSteps([]);
      setError(undefined);
      setOutputs(undefined);
      runRef.current = undefined;

      void (async () => {
        try {
          const newRun = await client.workflows.run(workflowId, options);
          if (generationRef.current !== generation) return;
          runRef.current = newRun;
          setRun(newRun);

          let last: RunSnapshot | undefined;
          for await (const snapshot of newRun.watch({ signal: controller.signal })) {
            if (generationRef.current !== generation) return;
            setSteps(snapshot.steps);
            last = snapshot;
          }
          if (generationRef.current !== generation || !last) return;

          setError(last.error);
          setOutputs(last.outputs);
          setPhase(last.status === "succeeded" ? "succeeded" : last.status === "cancelled" ? "cancelled" : "failed");
        } catch (err) {
          if (generationRef.current !== generation) return;
          if (err instanceof DOMException && err.name === "AbortError") return; // torn down deliberately, not a real failure
          setPhase("failed");
          setError(err instanceof Error ? err.message : String(err));
        }
      })();
    },
    [client, workflowId],
  );

  const cancel = useCallback(() => {
    void runRef.current?.cancel().catch(() => {
      // The run may already have reached a terminal status by the time
      // cancel() is called — the watch() loop still running reflects
      // whatever that real terminal status turns out to be either way.
    });
  }, []);

  const reset = useCallback(() => {
    generationRef.current += 1; // invalidate any in-flight loop first
    controllerRef.current?.abort();
    runRef.current = undefined;
    setPhase("idle");
    setRun(undefined);
    setSteps([]);
    setError(undefined);
    setOutputs(undefined);
  }, []);

  return { phase, run, steps, error, outputs, start, cancel, reset };
}
