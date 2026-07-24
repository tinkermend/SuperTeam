import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";

export const Route = createFileRoute("/_authenticated/task-launches/$demandId")({
  component: TaskLaunchDetailRedirectRoute
});

function TaskLaunchDetailRedirectRoute() {
  const { demandId } = Route.useParams();
  const navigate = useNavigate();

  useEffect(() => {
    void navigate({
      params: { demandId },
      replace: true,
      to: "/workflows/$demandId"
});
  }, [demandId, navigate]);

  return <div className="text-sm text-muted-foreground">正在打开流程编排</div>;
}
