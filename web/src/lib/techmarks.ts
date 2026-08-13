// Real brand marks for "See also" cards backed by a `skills` facet — the
// technology/cloud/framework side of FILTER_COLLECTIONS (see collections.ts).
// Mirrors backers.ts: a flat, hand-maintained map from our collection slug to
// a verified source, not an auto-derived guess. Source here is the
// `simple-icons` package (official brand SVG path + brand hex per icon),
// imported by name so unused icons are tree-shaken.
//
// Coverage is intentionally partial: `simple-icons` does not carry AWS,
// Azure, Java, or C# (removed following trademark takedown requests from
// their owners), so those four `skills` slugs have no entry here and render
// the generic tech family icon instead — the same graceful fallback any
// future skill gets before someone adds it below.
import {
  siAngular,
  siAnsible,
  siApachekafka,
  siClojure,
  siCplusplus,
  siDjango,
  siDocker,
  siDotnet,
  siElasticsearch,
  siElixir,
  siFlutter,
  siGo,
  siGooglecloud,
  siGraphql,
  siJavascript,
  siJenkins,
  siKotlin,
  siKubernetes,
  siLaravel,
  siMongodb,
  siMysql,
  siNextdotjs,
  siNodedotjs,
  siPhp,
  siPostgresql,
  siPython,
  siPytorch,
  siReact,
  siRedis,
  siRuby,
  siRubyonrails,
  siRust,
  siScala,
  siSolidity,
  siSpring,
  siSvelte,
  siSwift,
  siTensorflow,
  siTerraform,
  siTypescript,
  siVuedotjs,
} from 'simple-icons';

export type TechMark = {
  title: string;
  path: string;
  hex: string;
};

export const TECH_MARKS: Record<string, TechMark> = {
  python: siPython,
  javascript: siJavascript,
  typescript: siTypescript,
  cpp: siCplusplus,
  go: siGo,
  rust: siRust,
  ruby: siRuby,
  php: siPhp,
  kotlin: siKotlin,
  swift: siSwift,
  scala: siScala,
  nodejs: siNodedotjs,
  clojure: siClojure,
  elixir: siElixir,
  react: siReact,
  angular: siAngular,
  vue: siVuedotjs,
  nextjs: siNextdotjs,
  spring: siSpring,
  rails: siRubyonrails,
  django: siDjango,
  svelte: siSvelte,
  kubernetes: siKubernetes,
  terraform: siTerraform,
  docker: siDocker,
  postgresql: siPostgresql,
  redis: siRedis,
  kafka: siApachekafka,
  graphql: siGraphql,
  mongodb: siMongodb,
  mysql: siMysql,
  elasticsearch: siElasticsearch,
  dotnet: siDotnet,
  laravel: siLaravel,
  flutter: siFlutter,
  gcp: siGooglecloud,
  jenkins: siJenkins,
  ansible: siAnsible,
  pytorch: siPytorch,
  tensorflow: siTensorflow,
  solidity: siSolidity,
};
