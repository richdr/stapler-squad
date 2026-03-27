import { configureStore } from "@reduxjs/toolkit";
import approvalsReducer, {
  setApprovals,
  setLoading,
  setError,
  removeApproval,
  selectApprovals,
  selectApprovalsLoading,
  selectApprovalsError,
} from "../approvalsSlice";
import { PendingApprovalProto } from "@/gen/session/v1/types_pb";

function makeStore() {
  return configureStore({
    reducer: { approvals: approvalsReducer },
    middleware: (getDefault) => getDefault({ serializableCheck: false }),
  });
}

function makeApproval(id: string): PendingApprovalProto {
  return new PendingApprovalProto({ id, sessionId: `session-${id}` });
}

describe("approvalsSlice", () => {
  describe("initial state", () => {
    it("starts with empty approvals, not loading, no error", () => {
      const store = makeStore();
      const state = store.getState();
      expect(selectApprovals(state as any)).toEqual([]);
      expect(selectApprovalsLoading(state as any)).toBe(false);
      expect(selectApprovalsError(state as any)).toBeNull();
    });
  });

  describe("setApprovals", () => {
    it("replaces the approvals list", () => {
      const store = makeStore();
      const approvals = [makeApproval("a1"), makeApproval("a2")];
      store.dispatch(setApprovals(approvals));
      expect(selectApprovals(store.getState() as any)).toHaveLength(2);
      expect(selectApprovals(store.getState() as any)[0].id).toBe("a1");
    });

    it("replaces existing approvals on subsequent calls", () => {
      const store = makeStore();
      store.dispatch(setApprovals([makeApproval("old")]));
      store.dispatch(setApprovals([makeApproval("new1"), makeApproval("new2")]));
      const approvals = selectApprovals(store.getState() as any);
      expect(approvals).toHaveLength(2);
      expect(approvals[0].id).toBe("new1");
    });

    it("accepts an empty array to clear approvals", () => {
      const store = makeStore();
      store.dispatch(setApprovals([makeApproval("a1")]));
      store.dispatch(setApprovals([]));
      expect(selectApprovals(store.getState() as any)).toHaveLength(0);
    });
  });

  describe("removeApproval (optimistic update)", () => {
    it("removes the approval with the matching id", () => {
      const store = makeStore();
      store.dispatch(setApprovals([makeApproval("a1"), makeApproval("a2"), makeApproval("a3")]));
      store.dispatch(removeApproval("a2"));
      const approvals = selectApprovals(store.getState() as any);
      expect(approvals).toHaveLength(2);
      expect(approvals.map((a) => a.id)).toEqual(["a1", "a3"]);
    });

    it("is a no-op when the id does not exist", () => {
      const store = makeStore();
      store.dispatch(setApprovals([makeApproval("a1")]));
      store.dispatch(removeApproval("nonexistent"));
      expect(selectApprovals(store.getState() as any)).toHaveLength(1);
    });

    it("correctly removes the last item", () => {
      const store = makeStore();
      store.dispatch(setApprovals([makeApproval("only")]));
      store.dispatch(removeApproval("only"));
      expect(selectApprovals(store.getState() as any)).toHaveLength(0);
    });
  });

  describe("setLoading", () => {
    it("sets loading to true", () => {
      const store = makeStore();
      store.dispatch(setLoading(true));
      expect(selectApprovalsLoading(store.getState() as any)).toBe(true);
    });

    it("sets loading back to false", () => {
      const store = makeStore();
      store.dispatch(setLoading(true));
      store.dispatch(setLoading(false));
      expect(selectApprovalsLoading(store.getState() as any)).toBe(false);
    });
  });

  describe("setError", () => {
    it("stores an error message", () => {
      const store = makeStore();
      store.dispatch(setError("fetch failed"));
      expect(selectApprovalsError(store.getState() as any)).toBe("fetch failed");
    });

    it("clears the error with null", () => {
      const store = makeStore();
      store.dispatch(setError("some error"));
      store.dispatch(setError(null));
      expect(selectApprovalsError(store.getState() as any)).toBeNull();
    });
  });

  describe("optimistic update + rollback pattern", () => {
    it("restores state after rollback via setApprovals", () => {
      const store = makeStore();
      const initial = [makeApproval("a1"), makeApproval("a2")];
      store.dispatch(setApprovals(initial));

      // Simulate optimistic remove
      store.dispatch(removeApproval("a1"));
      expect(selectApprovals(store.getState() as any)).toHaveLength(1);

      // Simulate rollback (API failed, re-fetch restored original list)
      store.dispatch(setApprovals(initial));
      expect(selectApprovals(store.getState() as any)).toHaveLength(2);
    });
  });
});
