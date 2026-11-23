"use client";

import { useParams } from "next/navigation";

import { ClassLabs } from "../../../../components/live/class-screens";

export default function LiveClassPage() {
  const params = useParams<{ classId: string }>();
  const classId = typeof params.classId === "string" ? params.classId : "";
  if (classId.length === 0) return <p className="live-muted">This class address is not valid.</p>;
  return <ClassLabs classId={classId} />;
}
