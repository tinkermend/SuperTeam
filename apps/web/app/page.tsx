import { createHealthSummary } from "@superteam/core";

import { HomePage } from "../src/home-page";

export default function Page() {
  const summary = createHealthSummary({
    status: "ok",
    service: "control-plane",
  });

  return <HomePage summary={summary} />;
}
