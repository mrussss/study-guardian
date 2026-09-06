import { useCallback, useEffect, useReducer } from "react";

export type TaskSelectionState = {
  authoritativeTask: string;
  optimisticTask?: string;
  pending: boolean;
};

export type TaskSelectionAction =
  | { type: "SNAPSHOT_RECEIVED"; task: string }
  | { type: "OPTIMISTIC_SELECTED"; task: string }
  | { type: "MUTATION_SUCCEEDED"; task?: string }
  | { type: "MUTATION_FAILED" };

export function taskSelectionReducer(state: TaskSelectionState, action: TaskSelectionAction): TaskSelectionState {
  switch (action.type) {
    case "SNAPSHOT_RECEIVED":
      if (!state.pending) return { authoritativeTask: action.task, pending: false };
      if (state.optimisticTask === action.task) return { authoritativeTask: action.task, pending: false };
      return { ...state, authoritativeTask: action.task };
    case "OPTIMISTIC_SELECTED":
      return { ...state, optimisticTask: action.task, pending: true };
    case "MUTATION_SUCCEEDED": {
      const confirmedTask = action.task ?? state.optimisticTask ?? state.authoritativeTask;
      return { authoritativeTask: confirmedTask, pending: false };
    }
    case "MUTATION_FAILED":
      return { authoritativeTask: state.authoritativeTask, pending: false };
  }
}

export function useTaskSelectionState(snapshotTask: string): {
  task: string;
  state: TaskSelectionState;
  selectOptimistically: (task: string) => void;
  settle: (ok: boolean, task?: string) => void;
} {
  const [state, dispatch] = useReducer(taskSelectionReducer, {
    authoritativeTask: snapshotTask,
    pending: false,
  });
  useEffect(() => { dispatch({ type: "SNAPSHOT_RECEIVED", task: snapshotTask }); }, [snapshotTask]);
  const selectOptimistically = useCallback((task: string) => { dispatch({ type: "OPTIMISTIC_SELECTED", task }); }, []);
  const settle = useCallback((ok: boolean, task?: string) => {
    dispatch(ok ? { type: "MUTATION_SUCCEEDED", task } : { type: "MUTATION_FAILED" });
  }, []);
  return { task: state.optimisticTask ?? state.authoritativeTask, state, selectOptimistically, settle };
}
