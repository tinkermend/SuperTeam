import { useSyncExternalStore } from "react";
import {
  getInboxStreamStatus,
  subscribeInboxStreamStatus,
  type InboxStreamStatus,
} from "./inbox-stream-status";

export function useInboxStreamStatus(): InboxStreamStatus {
  return useSyncExternalStore(
    subscribeInboxStreamStatus,
    getInboxStreamStatus,
    getInboxStreamStatus,
  );
}
