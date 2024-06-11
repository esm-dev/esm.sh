import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import { graphql, GraphQLObjectType, GraphQLSchema, GraphQLString } from "http://localhost:8080/graphql@16.6.0";
import * as graphqlImpl from "http://localhost:8080/graphql@16.6.0/graphql";

Deno.test("graphql", async () => {
  const schema = new GraphQLSchema({
    query: new GraphQLObjectType({
      name: "RootQueryType",
      fields: {
        hello: {
          type: GraphQLString,
          resolve() {
            return "world";
          },
        },
      },
    }),
  });
  const source = "{ hello }";
  const ret = await graphql({ schema, source });
  assertEquals(ret, { data: { hello: "world" } });
  assertEquals(Object.keys(graphqlImpl), ["graphql", "graphqlSync"]);
});
