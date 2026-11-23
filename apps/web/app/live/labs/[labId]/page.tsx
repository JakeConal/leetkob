"use client";

import { useParams } from "next/navigation";

import { StudentLab } from "../../../../components/live/student-lab";

export default function LiveLabPage() {
  const params = useParams<{ labId: string }>();
  const labId = typeof params.labId === "string" ? params.labId : "";
  if (labId.length === 0) return <p className="live-muted">This lab address is not valid.</p>;
  return <StudentLab labId={labId} />;
}
