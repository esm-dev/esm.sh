// from @talentlessguy
export default v => (typeof v === "number" && v - v === 0) || (typeof v === "string" && Number.isFinite(+v) && v.trim() !== "");
