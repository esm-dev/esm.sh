import { assert, assertEquals } from "jsr:@std/assert";

// Needed as some Angular classes would need to be compiled.
import "http://localhost:8080/@angular/compiler@20.1.4";

import {
  FactoryProvider,
  Provider,
  RendererFactory2,
} from "http://localhost:8080/@angular/core@20.1.4";
import { ɵDomRendererFactory2 } from "http://localhost:8080/@angular/platform-browser@20.1.4";
import { provideAnimations } from "http://localhost:8080/@angular/platform-browser@20.1.4/animations";

// related issue: https://github.com/esm-dev/esm.sh/issues/1178
Deno.test(
  "testing identity of classes matches between entry-points",
  () => {
    const renderFactoryProvider = provideAnimations()
      .find((r: Provider): r is FactoryProvider =>
        (r as Partial<FactoryProvider>).provide === RendererFactory2
      );
    assert(
      renderFactoryProvider !== undefined,
      "Expected renderer factory provider to be found.",
    );
    assertEquals(
      renderFactoryProvider.deps?.[0],
      ɵDomRendererFactory2,
      "Expected identity of DomRendererFactory to match between entry-points.",
    );
  },
);
