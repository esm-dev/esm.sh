// @ts-nocheck

import { assertEquals } from "https://deno.land/std@0.178.0/testing/asserts.ts";
import * as tf from "http://localhost:8080/@tensorflow/tfjs?no-dts";

Deno.test("tensorflow", async () => {
  // Define a model for linear regression.
  const model = tf.sequential();
  model.add(tf.layers.dense({ units: 1, inputShape: [1] }));

  // Prepare the model for training: Specify the loss and the optimizer.
  model.compile({ loss: "meanSquaredError", optimizer: "sgd" });

  // Generate some synthetic data for training.
  const xs = tf.tensor2d([1, 2, 3, 4], [4, 1]);
  const ys = tf.tensor2d([1, 3, 5, 7], [4, 1]);

  // Train the model using the data.
  await model.fit(xs, ys);

  // Use the model to do inference on a data point the model hasn't seen before:
  // Open the browser devtools to see the output
  const output = model.predict(tf.tensor2d([5], [1, 1]));
  const values = output.arraySync();

  assertEquals(output.shape, [1, 1]);
  assertEquals(typeof values[0][0], "number");
});
