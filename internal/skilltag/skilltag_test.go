package skilltag

import (
	"reflect"
	"testing"
)

func TestNormalizeStripsHTMLAndLowercases(t *testing.T) {
	got := normalize("<div><p>Senior <b>Go</b> Engineer</p></div>")
	// Two spaces between words: each surrounding tag is replaced by one space.
	want := "senior  go  engineer"
	if got != want {
		t.Fatalf("normalize = %q, want %q", got, want)
	}
}

func TestWordTokens(t *testing.T) {
	got := wordTokens("go, node.js & c++17")
	want := []string{"go", "node", "js", "c", "17"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wordTokens = %#v, want %#v", got, want)
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"plain alias", "We use Golang and PostgreSQL.", []string{"go", "postgresql"}},
		{"dedup + sort", "react, React.js, REACT", []string{"react"}},
		{"punctuated", "Strong C++ and C# with .NET.", []string{"cpp", "csharp", "dotnet"}},
		{"node variants", "node.js / nodejs / node js", []string{"nodejs"}},
		{"multiword", "React Native and CI/CD pipelines", []string{"ci-cd", "react", "react-native"}},
		{"ambiguous word rejected", "Please go to the careers page in C.", nil},
		{"ambiguous via qualifier", "5y as a C developer", []string{"c"}},
		{"objective-c does not leak bare c", "Objective-C developer", []string{"objective-c"}},
		{"objective-c alongside real C still tags both", "Objective-C and C developer", []string{"c", "objective-c"}},
		{"word boundary", "a reaction to going rusty", nil},
		{"html stripped", "<p>Kubernetes</p><a href='k8s'>k8s</a>", []string{"kubernetes"}},
		{"empty", "", nil},
		{"dotted domain not matched", "see docs at foo.asp.net for details", nil},
		{"sentence-end periods ok", "We use C#. Also ASP.NET.", []string{"csharp", "dotnet"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// New tech/methodology vocabulary (skills-vocab-gaps): each resolves from a
// realistic description; bare "rest" never tags (only "REST API"/"RESTful").
func TestParse_NewTechVocab(t *testing.T) {
	contains := func(hay []string, needle string) bool {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}
	cases := []struct {
		name   string
		in     string
		want   []string // all must be present
		absent []string // none may be present
	}{
		{"methodologies", "We work in an Agile environment using Scrum and Kanban.", []string{"agile", "scrum", "kanban"}, nil},
		{"platforms", "Experience with Salesforce, SAP and Oracle is required.", []string{"salesforce", "sap", "oracle"}, nil},
		{"practices", "You will own our DevOps and observability across microservices.", []string{"devops", "observability", "microservices"}, nil},
		{"microservice singular", "Designing a microservice architecture.", []string{"microservices"}, nil},
		{"rest via restful", "Build RESTful APIs in Go.", []string{"rest"}, nil},
		{"rest via rest api", "You will design a REST API.", []string{"rest"}, nil},
		{"power bi", "Dashboards in Power BI.", []string{"powerbi"}, nil},
		{"rest trap", "You will support the rest of the team and ship features.", nil, []string{"rest"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.in)
			for _, w := range c.want {
				if !contains(got, w) {
					t.Errorf("Parse(%q) = %v, missing %q", c.in, got, w)
				}
			}
			for _, a := range c.absent {
				if contains(got, a) {
					t.Errorf("Parse(%q) = %v, must NOT contain %q", c.in, got, a)
				}
			}
		})
	}
}

// AI/LLM engineering vocabulary (ai-skills-gaps): vector DBs, LLM frameworks,
// model providers, and serving/inference tools resolve from realistic text.
// Ambiguous bare words are deliberately NOT tagged (RAG project status, the word
// "bedrock", the "needle in a haystack" idiom) — those collide with non-AI jobs.
func TestParse_AIVocab(t *testing.T) {
	contains := func(hay []string, needle string) bool {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		{"vector dbs", "Vector search over Pinecone, Weaviate, Qdrant, Milvus, pgvector, FAISS and ChromaDB.",
			[]string{"pinecone", "weaviate", "qdrant", "milvus", "pgvector", "faiss", "chromadb"}, nil},
		{"chroma db spaced", "We store embeddings in Chroma DB.", []string{"chromadb"}, nil},
		{"llm frameworks", "Orchestrate agents with LangGraph, LangSmith, LlamaIndex, CrewAI and AutoGen.",
			[]string{"langgraph", "langsmith", "llamaindex", "crewai", "autogen"}, nil},
		{"framework spacing", "Built on Llama Index and Crew AI.", []string{"llamaindex", "crewai"}, nil},
		{"semantic kernel", "Microsoft Semantic Kernel experience.", []string{"semantic-kernel"}, nil},
		{"providers", "Integrate Anthropic, Cohere, Mistral and Ollama models.",
			[]string{"anthropic", "cohere", "mistral", "ollama"}, nil},
		{"clouds", "Deploy on Databricks, AWS SageMaker, Vertex AI and AWS Bedrock.",
			[]string{"databricks", "sagemaker", "vertex-ai", "aws-bedrock"}, nil},
		{"amazon bedrock", "Using Amazon Bedrock for inference.", []string{"aws-bedrock"}, nil},
		{"serving", "Serve models with vLLM, Triton, TensorRT, ONNX and BentoML.",
			[]string{"vllm", "triton", "tensorrt", "onnx", "bentoml"}, nil},
		{"training", "Fine-tuning with DeepSpeed and PEFT; sentence-transformers for embeddings.",
			[]string{"deepspeed", "peft", "sentence-transformers"}, nil},
		{"cv model", "Object detection with YOLO.", []string{"yolo"}, nil},
		{"rag phrase", "Build retrieval augmented generation pipelines.", []string{"rag"}, nil},
		// ambiguity guards: these must NOT tag (skilltag runs on ALL jobs).
		{"rag status trap", "We report project health as RAG status weekly.", nil, []string{"rag"}},
		{"bedrock word trap", "Trust is the bedrock of our company culture.", nil, []string{"aws-bedrock"}},
		{"haystack idiom trap", "Finding signal is a needle in a haystack.", nil, []string{"haystack"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.in)
			for _, w := range c.want {
				if !contains(got, w) {
					t.Errorf("Parse(%q) = %v, missing %q", c.in, got, w)
				}
			}
			for _, a := range c.absent {
				if contains(got, a) {
					t.Errorf("Parse(%q) = %v, must NOT contain %q", c.in, got, a)
				}
			}
		})
	}
}
