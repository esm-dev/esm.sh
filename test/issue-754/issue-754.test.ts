import { assertEquals } from "https://deno.land/std@0.210.0/testing/asserts.ts";

import { fromLonLat } from "http://localhost:8080/ol@8.1.0/proj";
import Projection from "http://localhost:8080/ol@8.1.0/proj/Projection";
import { register } from "http://localhost:8080/ol@8.1.0/proj/proj4";
import proj4 from "http://localhost:8080/proj4@2.9.2";

Deno.test("issue #754", () => {
  const rdId = "EPSG:28992";
  proj4.defs(
    rdId,
    "+proj=sterea +lat_0=52.15616055555555 +lon_0=5.38763888888889 +k=0.9999079 +x_0=155000 +y_0=463000 +ellps=bessel +towgs84=565.417,50.3319,465.552,-0.398957,0.343988,-1.8774,4.0725 +units=m +no_defs",
  );
  register(proj4);
  const rdProjection = new Projection({
    code: rdId,
    extent: [-285401.92, 22598.08, 595401.92, 903401.92],
  });
  const rd_coord_proj = proj4("EPSG:4326", rdId, [5.537109, 52.342052]);
  const rd_coord_ol = fromLonLat([5.537109, 52.342052], rdProjection);
  assertEquals(rd_coord_proj, rd_coord_ol);
});
