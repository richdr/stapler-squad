import { configureStore } from "@reduxjs/toolkit";
import { useDispatch, useSelector } from "react-redux";
import approvalsReducer from "./approvalsSlice";
import reviewQueueReducer from "./reviewQueueSlice";
import sessionsReducer from "./sessionsSlice";

export const store = configureStore({
  reducer: {
    approvals: approvalsReducer,
    reviewQueue: reviewQueueReducer,
    sessions: sessionsReducer,
  },
  middleware: (getDefaultMiddleware) =>
    getDefaultMiddleware({
      // @bufbuild/protobuf v1 generates class instances (Session, ReviewQueue,
      // PendingApprovalProto) with non-enumerable internal fields that fail
      // Redux's serializable check. Rather than disabling the check globally,
      // we suppress it only for the specific state paths and actions that hold
      // protobuf objects. All other state paths retain full serialization
      // protection.
      //
      // Long-term fix: upgrade to @bufbuild/protobuf v2 (+ @connectrpc/connect
      // v2), where generated messages are plain TypeScript objects with no
      // prototype chain — fully compatible with Immer and Redux DevTools.
      serializableCheck: {
        ignoredPaths: [
          "sessions.entities",       // Session class instances (entity adapter map)
          "approvals.approvals",     // PendingApprovalProto[]
          "reviewQueue.reviewQueue", // ReviewQueue class instance
        ],
        ignoredActions: [
          "sessions/setSessions",
          "sessions/upsertSession",
          "sessions/updateSessionStatus",
          "approvals/setApprovals",
          "reviewQueue/setReviewQueue",
          "reviewQueue/removeItem",
        ],
      },
    }),
});

export type RootState = ReturnType<typeof store.getState>;
export type AppDispatch = typeof store.dispatch;

// Typed hooks for use throughout the app
export const useAppDispatch = useDispatch.withTypes<AppDispatch>();
export const useAppSelector = useSelector.withTypes<RootState>();
