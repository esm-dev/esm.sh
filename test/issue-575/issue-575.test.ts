import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";
import * as mod from "http://localhost:8080/@mui/material@5.11.15?alias=react:preact/compat,react-dom:preact/compat,react/jsx-runtime:preact/jsx-runtime&deps=preact@10.6.6&target=es2020&exports=Autocomplete,createFilterOptions,TextField,Button,Dialog,DialogTitle,DialogContent,DialogContentText,DialogActions,Accordion,AccordionSummary,AccordionDetails,Typography";

Deno.test("issue #575", () => {
  assertEquals(Object.keys(mod).sort(), [
    "Accordion",
    "AccordionDetails",
    "AccordionSummary",
    "Autocomplete",
    "Button",
    "Dialog",
    "DialogActions",
    "DialogContent",
    "DialogContentText",
    "DialogTitle",
    "TextField",
    "Typography",
    "createFilterOptions",
  ]);
});
