package commands

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/drover-org/drover-ontology/internal/ontology"
	"github.com/spf13/cobra"
)

var (
	visualizePort int
	visualizeHost string
)

var visualizeCmd = &cobra.Command{
	Use:   "visualize",
	Short: "Start an interactive web server to visualize the ontology in your browser",
	Long: `Launches a lightweight local web server (defaults to port 8080) and opens your
browser to display a beautiful, interactive force-directed graph of your active ontology.

You can inspect node properties, trace relationships, search terms, and refresh the
graph live as you update the database in other terminals.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ont := getOntology()
		if ont == nil {
			return fmt.Errorf("ontology not initialized")
		}

		// Find a free port if visualizePort is 0
		port := visualizePort
		if port == 0 {
			port = 8080
		}

		// Create listener to verify port availability and get actual port if 0 was passed
		addr := fmt.Sprintf("%s:%d", visualizeHost, port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			// If port is taken and port was not explicitly specified, let's find an ephemeral port
			if visualizePort == 0 {
				addr = fmt.Sprintf("%s:0", visualizeHost)
				listener, err = net.Listen("tcp", addr)
				if err != nil {
					return fmt.Errorf("failed to listen on ephemeral port: %w", err)
				}
			} else {
				return fmt.Errorf("failed to listen on %s: %w", addr, err)
			}
		}
		defer listener.Close()

		// Get the actual resolved address
		resolvedAddr := listener.Addr().String()
		parts := strings.Split(resolvedAddr, ":")
		actualPort := parts[len(parts)-1]
		url := fmt.Sprintf("http://127.0.0.1:%s", actualPort)

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			tmpl, err := template.New("index").Parse(htmlTemplate)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			projectRoot := os.Getenv("HOST_PROJECT_ROOT")
			if projectRoot == "" {
				projectRoot, _ = os.Getwd()
			}
			data := map[string]any{
				"ProjectRoot": projectRoot,
			}
			tmpl.Execute(w, data)
		})

		http.HandleFunc("/api/graph", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			
			// Dynamically reload from storage so that updates (e.g. from RLM runs or CLI) are instantly visualizable
			if err := ont.Reload(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to reload ontology from storage: %v\n", err)
			}
			
			// Filter out tombstoned or invalidated entities
			var activeEntities []*ontology.Entity
			for _, e := range ont.All() {
				if !ontology.IsDeleted(e) {
					activeEntities = append(activeEntities, e)
				}
			}

			// Format for frontend
			type APINode struct {
				ID         string         `json:"id"`
				Label      string         `json:"label"`
				Type       string         `json:"type"`
				Properties map[string]any `json:"properties"`
			}
			type APIEdge struct {
				From  string `json:"from"`
				To    string `json:"to"`
				Label string `json:"label"`
			}
			type APIGraph struct {
				Nodes []APINode `json:"nodes"`
				Edges []APIEdge `json:"edges"`
			}

			var nodes []APINode
			var edges []APIEdge

			activeIDs := make(map[string]bool)
			for _, e := range activeEntities {
				activeIDs[e.ID] = true
				
				// Clean label for presentation (e.g. "Term:asdlc/governance" -> "asdlc/governance")
				displayLabel := e.ID
				parts := strings.SplitN(e.ID, ":", 2)
				if len(parts) == 2 {
					displayLabel = parts[1]
				}

				nodes = append(nodes, APINode{
					ID:         e.ID,
					Label:      displayLabel,
					Type:       e.Type,
					Properties: e.Properties,
				})
			}

			for _, e := range activeEntities {
				for _, r := range e.Relations {
					if activeIDs[r.To] {
						edges = append(edges, APIEdge{
							From:  e.ID,
							To:    r.To,
							Label: r.Type,
						})
					}
				}
			}

			json.NewEncoder(w).Encode(APIGraph{Nodes: nodes, Edges: edges})
		})

		fmt.Printf("\n🚀 Starting drover ontology visualizer at %s\n", url)
		fmt.Println("Press Ctrl+C to stop the server.")

		// Launch the browser asynchronously
		go func() {
			openBrowser(url)
		}()

		// Start serving on the listener
		server := &http.Server{}
		return server.Serve(listener)
	},
}

func init() {
	visualizeCmd.Flags().IntVarP(&visualizePort, "port", "p", 8080, "Port to run the visualizer server on (0 for auto-assign)")
	visualizeCmd.Flags().StringVar(&visualizeHost, "host", "127.0.0.1", "Host IP to bind the visualizer server on")
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	if err != nil {
		fmt.Printf("Could not open browser automatically: %v\n", err)
		fmt.Printf("Please open this URL manually in your browser: %s\n", url)
	}
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Drover Ontology Visualizer</title>
    <!-- Outfit Font -->
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;500;600;700&display=swap" rel="stylesheet">
    
    <!-- Vis.js CDN -->
    <script type="text/javascript" src="https://unpkg.com/vis-network/standalone/umd/vis-network.min.js"></script>
    
    <!-- Lucide Icons -->
    <script src="https://unpkg.com/lucide@latest"></script>

    <style>
        :root {
            --bg-main: #0f172a;
            --bg-surface: #1e293b;
            --bg-surface-glass: rgba(30, 41, 59, 0.85);
            --border-color: rgba(255, 255, 255, 0.1);
            --text-primary: #f8fafc;
            --text-secondary: #94a3b8;
            --accent-color: #38bdf8;
            --accent-glow: rgba(56, 189, 248, 0.15);
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }

        body {
            font-family: 'Outfit', sans-serif;
            background-color: #0b0f19;
            color: var(--text-primary);
            height: 100vh;
            overflow: hidden;
            display: flex;
            flex-direction: column;
        }

        /* Top Header Bar */
        header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 1rem 2rem;
            background-color: var(--bg-main);
            border-bottom: 1px solid var(--border-color);
            z-index: 10;
            box-shadow: 0 4px 20px rgba(0, 0, 0, 0.25);
        }

        .logo-section {
            display: flex;
            align-items: center;
            gap: 0.75rem;
        }

        .logo-icon {
            color: var(--accent-color);
            display: flex;
            align-items: center;
            justify-content: center;
            background: var(--accent-glow);
            padding: 0.5rem;
            border-radius: 10px;
            border: 1px solid rgba(56, 189, 248, 0.2);
        }

        .logo-title h1 {
            font-size: 1.25rem;
            font-weight: 600;
            letter-spacing: -0.02em;
        }

        .logo-title p {
            font-size: 0.75rem;
            color: var(--text-secondary);
        }

        .controls-section {
            display: flex;
            align-items: center;
            gap: 1.25rem;
        }

        .search-container {
            position: relative;
            display: flex;
            align-items: center;
        }

        .search-input {
            background-color: rgba(0, 0, 0, 0.25);
            border: 1px solid var(--border-color);
            color: var(--text-primary);
            padding: 0.5rem 1rem 0.5rem 2.25rem;
            border-radius: 8px;
            font-family: inherit;
            font-size: 0.875rem;
            width: 250px;
            transition: all 0.25s cubic-bezier(0.16, 1, 0.3, 1);
        }

        .search-input:focus {
            outline: none;
            border-color: var(--accent-color);
            box-shadow: 0 0 0 3px var(--accent-glow);
            width: 350px;
        }

        .search-icon {
            position: absolute;
            left: 0.75rem;
            color: var(--text-secondary);
            pointer-events: none;
            width: 16px;
            height: 16px;
        }

        .btn {
            display: flex;
            align-items: center;
            gap: 0.5rem;
            background-color: rgba(255, 255, 255, 0.05);
            border: 1px solid var(--border-color);
            color: var(--text-primary);
            padding: 0.5rem 1rem;
            border-radius: 8px;
            font-family: inherit;
            font-size: 0.875rem;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.2s ease;
        }

        .btn:hover {
            background-color: rgba(255, 255, 255, 0.1);
            border-color: rgba(255, 255, 255, 0.2);
        }

        .status-badge {
            background-color: rgba(255, 255, 255, 0.05);
            border: 1px solid var(--border-color);
            padding: 0.35rem 0.85rem;
            border-radius: 20px;
            font-size: 0.75rem;
            font-weight: 500;
            color: var(--text-secondary);
            display: flex;
            align-items: center;
            gap: 0.45rem;
        }

        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background-color: #10b981;
            box-shadow: 0 0 8px #10b981;
        }

        /* Main Content Layout */
        main {
            display: flex;
            flex: 1;
            position: relative;
            overflow: hidden;
        }

        /* Graph Network Area */
        #network-container {
            flex: 1;
            height: 100%;
            position: relative;
            background-color: #0b0f19;
        }

        /* Side Inspector Drawer */
        #inspector-drawer {
            width: 420px;
            background: var(--bg-surface-glass);
            backdrop-filter: blur(16px);
            border-left: 1px solid var(--border-color);
            display: flex;
            flex-direction: column;
            height: 100%;
            position: relative;
            z-index: 5;
            box-shadow: -10px 0 35px rgba(0, 0, 0, 0.4);
            transition: transform 0.35s cubic-bezier(0.16, 1, 0.3, 1);
            transform: translateX(0);
        }

        #inspector-drawer.collapsed {
            transform: translateX(100%);
            position: absolute;
            right: 0;
        }

        .drawer-header {
            padding: 1.5rem;
            border-bottom: 1px solid var(--border-color);
            display: flex;
            align-items: center;
            justify-content: space-between;
        }

        .drawer-title {
            font-size: 1.1rem;
            font-weight: 600;
            letter-spacing: -0.01em;
        }

        .close-btn {
            background: none;
            border: none;
            color: var(--text-secondary);
            cursor: pointer;
            padding: 0.25rem;
            border-radius: 6px;
            display: flex;
            align-items: center;
            justify-content: center;
            transition: all 0.2s ease;
        }

        .close-btn:hover {
            background-color: rgba(255, 255, 255, 0.05);
            color: var(--text-primary);
        }

        .drawer-body {
            flex: 1;
            overflow-y: auto;
            padding: 1.5rem;
            display: flex;
            flex-direction: column;
            gap: 1.75rem;
        }

        .empty-inspector {
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            height: 100%;
            color: var(--text-secondary);
            text-align: center;
            gap: 1rem;
            padding: 2rem;
        }

        .empty-icon {
            width: 48px;
            height: 48px;
            color: rgba(255, 255, 255, 0.1);
        }

        /* Inspector Details Styling */
        .entity-header {
            background-color: rgba(255, 255, 255, 0.02);
            border: 1px solid var(--border-color);
            border-radius: 12px;
            padding: 1.25rem;
            display: flex;
            flex-direction: column;
            gap: 0.6rem;
        }

        .entity-type-badge {
            align-self: flex-start;
            padding: 0.25rem 0.65rem;
            border-radius: 20px;
            font-size: 0.675rem;
            font-weight: 700;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }

        .entity-id {
            font-size: 1.2rem;
            font-weight: 600;
            word-break: break-all;
            line-height: 1.3;
        }

        .section-title {
            font-size: 0.825rem;
            font-weight: 600;
            color: var(--text-secondary);
            text-transform: uppercase;
            letter-spacing: 0.08em;
            margin-bottom: 0.85rem;
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }

        .properties-table {
            width: 100%;
            border-collapse: collapse;
            font-size: 0.875rem;
            background-color: rgba(0, 0, 0, 0.15);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            overflow: hidden;
        }

        .properties-table tr {
            border-bottom: 1px solid var(--border-color);
        }

        .properties-table tr:last-child {
            border-bottom: none;
        }

        .properties-table td {
            padding: 0.85rem 1rem;
            vertical-align: top;
        }

        .properties-key {
            font-weight: 500;
            color: var(--text-secondary);
            width: 35%;
            word-break: break-all;
            border-right: 1px solid var(--border-color);
        }

        .properties-val {
            color: var(--text-primary);
            word-break: break-all;
            padding-left: 1.25rem !important;
        }

        .relations-list {
            display: flex;
            flex-direction: column;
            gap: 0.6rem;
        }

        .relation-item {
            background-color: rgba(255, 255, 255, 0.02);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 0.85rem 1rem;
            display: flex;
            flex-direction: column;
            gap: 0.45rem;
            transition: all 0.2s ease;
        }

        .relation-item:hover {
            border-color: rgba(56, 189, 248, 0.35);
            background-color: rgba(56, 189, 248, 0.03);
        }

        .relation-type {
            font-size: 0.675rem;
            font-weight: 700;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            color: var(--accent-color);
        }

        .relation-target {
            font-size: 0.875rem;
            font-weight: 500;
            cursor: pointer;
            word-break: break-all;
            display: flex;
            align-items: center;
            gap: 0.45rem;
            color: var(--text-primary);
        }

        .relation-target:hover {
            color: var(--accent-color);
            text-decoration: underline;
        }

        /* Floating Info overlay / Legend */
        #legend-panel {
            position: absolute;
            bottom: 1.5rem;
            left: 1.5rem;
            background: var(--bg-surface-glass);
            backdrop-filter: blur(16px);
            border: 1px solid var(--border-color);
            border-radius: 12px;
            padding: 1.25rem;
            display: flex;
            flex-direction: column;
            gap: 0.85rem;
            z-index: 4;
            box-shadow: 0 10px 30px rgba(0, 0, 0, 0.3);
            max-width: 340px;
        }

        .legend-title {
            font-size: 0.75rem;
            font-weight: 700;
            text-transform: uppercase;
            letter-spacing: 0.08em;
            color: var(--text-secondary);
        }

        .legend-items {
            display: flex;
            flex-wrap: wrap;
            gap: 0.6rem;
        }

        .legend-item {
            display: flex;
            align-items: center;
            gap: 0.45rem;
            font-size: 0.75rem;
            background-color: rgba(255, 255, 255, 0.03);
            border: 1px solid var(--border-color);
            padding: 0.3rem 0.6rem;
            border-radius: 6px;
            font-weight: 500;
        }

        .legend-color {
            width: 12px;
            height: 12px;
            border-radius: 4px;
        }

        /* Toggle switches */
        .toggle-container {
            display: flex;
            align-items: center;
            gap: 0.6rem;
            font-size: 0.875rem;
            cursor: pointer;
            user-select: none;
            color: var(--text-secondary);
            font-weight: 500;
        }

        .toggle-container:hover {
            color: var(--text-primary);
        }

        .switch {
            position: relative;
            display: inline-block;
            width: 40px;
            height: 22px;
        }

        .switch input {
            opacity: 0;
            width: 0;
            height: 0;
        }

        .slider {
            position: absolute;
            cursor: pointer;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background-color: rgba(255, 255, 255, 0.1);
            transition: .25s cubic-bezier(0.16, 1, 0.3, 1);
            border-radius: 20px;
            border: 1px solid var(--border-color);
        }

        .slider:before {
            position: absolute;
            content: "";
            height: 14px;
            width: 14px;
            left: 3px;
            bottom: 3px;
            background-color: #94a3b8;
            transition: .25s cubic-bezier(0.16, 1, 0.3, 1);
            border-radius: 50%;
        }

        input:checked + .slider {
            background-color: var(--accent-glow);
            border-color: var(--accent-color);
        }

        input:checked + .slider:before {
            transform: translateX(18px);
            background-color: var(--accent-color);
            box-shadow: 0 0 8px var(--accent-color);
        }

        /* Code Lineage / Provenance Section */
        .provenance-card {
            background: linear-gradient(135deg, rgba(16, 185, 129, 0.05), rgba(56, 189, 248, 0.03));
            border: 1px solid rgba(16, 185, 129, 0.2);
            border-radius: 12px;
            padding: 1.15rem;
            display: flex;
            flex-direction: column;
            gap: 0.75rem;
            box-shadow: 0 8px 32px rgba(0, 0, 0, 0.2);
            transition: all 0.3s ease;
        }

        .provenance-card:hover {
            border-color: rgba(56, 189, 248, 0.5);
            box-shadow: 0 8px 32px rgba(56, 189, 248, 0.1);
        }

        .provenance-header-row {
            display: flex;
            align-items: center;
            justify-content: space-between;
        }

        .provenance-link {
            display: flex;
            align-items: center;
            gap: 0.5rem;
            color: var(--accent-color);
            font-size: 0.875rem;
            font-weight: 500;
            text-decoration: none;
            word-break: break-all;
            line-height: 1.4;
            transition: all 0.2s ease;
        }

        .provenance-link:hover {
            color: var(--text-primary);
            text-decoration: underline;
        }

        .lineage-badge {
            background-color: #10b981;
            color: #0b0f19;
            padding: 0.2rem 0.5rem;
            border-radius: 6px;
            font-size: 0.725rem;
            font-weight: 700;
            letter-spacing: 0.02em;
            display: inline-flex;
            align-items: center;
            box-shadow: 0 0 10px rgba(16, 185, 129, 0.3);
        }
    </style>
</head>
<body>
    <header>
        <div class="logo-section">
            <div class="logo-icon">
                <i data-lucide="network"></i>
            </div>
            <div class="logo-title">
                <h1>Drover Ontology</h1>
                <p>Hybrid Knowledge Graph & Taxonomy</p>
            </div>
        </div>

        <div class="controls-section">
            <div class="search-container">
                <i data-lucide="search" class="search-icon"></i>
                <input type="text" id="search-input" class="search-input" placeholder="Search nodes by ID...">
            </div>

            <label class="toggle-container">
                <span>Interactive Physics</span>
                <label class="switch">
                    <input type="checkbox" id="physics-toggle" checked>
                    <span class="slider"></span>
                </label>
            </label>

            <button class="btn" id="refresh-btn">
                <i data-lucide="refresh-cw" style="width: 16px; height: 16px;"></i>
                Sync Data
            </button>

            <div class="status-badge">
                <span class="status-dot"></span>
                <span id="stats-badge">Loading...</span>
            </div>
        </div>
    </header>

    <main>
        <!-- Graph Area -->
        <div id="network-container"></div>

        <!-- Legend Overlay -->
        <div id="legend-panel">
            <div class="legend-title">Entity Types</div>
            <div class="legend-items" id="legend-items-container"></div>
        </div>

        <!-- Side Drawer -->
        <div id="inspector-drawer" class="collapsed">
            <div class="drawer-header">
                <span class="drawer-title">Inspector</span>
                <button class="close-btn" id="close-drawer-btn">
                    <i data-lucide="x" style="width: 20px; height: 20px;"></i>
                </button>
            </div>
            <div class="drawer-body" id="drawer-body-container">
                <div class="empty-inspector">
                    <i data-lucide="info" class="empty-icon"></i>
                    <p>Select any node in the graph to inspect its properties and local relationships.</p>
                </div>
            </div>
        </div>
    </main>

    <script>
        const projectRoot = '{{ .ProjectRoot }}';
        // Set up custom Vis.js colors matching our premium aesthetics
        const colorMap = {
            bead: { background: '#D5F5E3', border: '#abebc6', highlight: { background: '#D5F5E3', border: '#2ecc71' } },
            task: { background: '#D5F5E3', border: '#abebc6', highlight: { background: '#D5F5E3', border: '#2ecc71' } },
            plan: { background: '#FADBD8', border: '#f5b7b1', highlight: { background: '#FADBD8', border: '#e74c3c' } },
            term: { background: '#E8F8F5', border: '#a3e4d7', highlight: { background: '#E8F8F5', border: '#1abc9c' } },
            concept: { background: '#EAECEE', border: '#d5dbdb', highlight: { background: '#EAECEE', border: '#95a5a6' } },
            person: { background: '#FCF3CF', border: '#f9e79f', highlight: { background: '#FCF3CF', border: '#f1c40f' } },
            review: { background: '#E8DAEF', border: '#d7bde2', highlight: { background: '#E8DAEF', border: '#9b59b6' } },
            default: { background: '#EBF5FB', border: '#aed6f1', highlight: { background: '#EBF5FB', border: '#3498db' } }
        };

        // Initialize Lucide icons
        lucide.createIcons();

        let network = null;
        let nodesDataset = new vis.DataSet([]);
        let edgesDataset = new vis.DataSet([]);
        let graphData = { nodes: [], edges: [] };

        // Fetch data from the API
        async function fetchGraphData() {
            try {
                const res = await fetch('/api/graph');
                if (!res.ok) throw new Error('API failed');
                
                const data = await res.json();
                graphData = data;
                
                // Process nodes to assign correct colors
                const formattedNodes = data.nodes.map(n => {
                    const typeLower = n.type.toLowerCase();
                    const color = colorMap[typeLower] || colorMap['default'];
                    return {
                        id: n.id,
                        label: n.label,
                        title: n.id + ' (' + n.type + ')',
                        color: color,
                        borderWidth: 2,
                        shape: 'box',
                        margin: 12,
                        font: {
                            face: 'Outfit',
                            color: '#0f172a',
                            size: 14,
                            bold: {
                                color: '#0f172a'
                            }
                        }
                    };
                });

                // Set datasets
                nodesDataset.clear();
                edgesDataset.clear();
                nodesDataset.add(formattedNodes);
                edgesDataset.add(data.edges);

                // Update Stats Badge
                document.getElementById('stats-badge').innerText = data.nodes.length + ' Nodes, ' + data.edges.length + ' Edges';

                // Render Legend
                renderLegend(data.nodes);
                
                return true;
            } catch (err) {
                console.error('Error fetching graph data:', err);
                document.getElementById('stats-badge').innerText = 'Sync Error';
                return false;
            }
        }

        function renderLegend(nodes) {
            const types = [...new Set(nodes.map(n => n.type))];
            const legendContainer = document.getElementById('legend-items-container');
            legendContainer.innerHTML = '';
            
            types.forEach(t => {
                const color = colorMap[t.toLowerCase()] || colorMap['default'];
                const item = document.createElement('div');
                item.className = 'legend-item';
                item.innerHTML = '<span class="legend-color" style="background-color: ' + color.background + '; border: 1px solid ' + color.border + '"></span>' + t;
                legendContainer.appendChild(item);
            });
        }

        // Initialize Vis Network
        function initNetwork() {
            const container = document.getElementById('network-container');
            const data = {
                nodes: nodesDataset,
                edges: edgesDataset
            };

            const options = {
                nodes: {
                    shadow: {
                        enabled: true,
                        color: 'rgba(0,0,0,0.4)',
                        size: 10,
                        x: 0,
                        y: 5
                    }
                },
                edges: {
                    width: 2,
                    color: {
                        color: 'rgba(255,255,255,0.18)',
                        highlight: '#38bdf8',
                        hover: 'rgba(255,255,255,0.4)'
                    },
                    arrows: {
                        to: {
                            enabled: true,
                            scaleFactor: 0.8
                        }
                    },
                    font: {
                        face: 'Outfit',
                        color: '#94a3b8',
                        size: 10,
                        align: 'middle',
                        strokeWidth: 0
                    },
                    smooth: {
                        type: 'continuous',
                        roundness: 0.4
                    }
                },
                physics: {
                    solver: 'forceAtlas2Based',
                    forceAtlas2Based: {
                        gravitationalConstant: -120,
                        centralGravity: 0.015,
                        springLength: 130,
                        springConstant: 0.08,
                        damping: 0.45,
                        avoidOverlap: 1
                    },
                    stabilization: {
                        iterations: 150
                    }
                },
                interaction: {
                    hover: true,
                    tooltipDelay: 200,
                    navigationButtons: true,
                    keyboard: true
                }
            };

            network = new vis.Network(container, data, options);

            // Handle Node Clicks
            network.on("selectNode", function (params) {
                if (params.nodes.length > 0) {
                    const nodeId = params.nodes[0];
                    inspectNode(nodeId);
                }
            });

            network.on("deselectNode", function (params) {
                closeDrawer();
            });
        }

        // Inspect a Node and render in the Side Inspector
        function inspectNode(nodeId) {
            const node = graphData.nodes.find(n => n.id === nodeId);
            if (!node) return;

            const typeLower = node.type.toLowerCase();
            const color = colorMap[typeLower] || colorMap['default'];
            
            const drawer = document.getElementById('inspector-drawer');
            const drawerBody = document.getElementById('drawer-body-container');
            
            // Build Code Lineage card if src_path is present
            let provenanceHTML = '';
            const props = node.properties || {};
            if (props.src_path) {
                const srcPath = String(props.src_path);
                const lineStart = props.line_start ? String(props.line_start) : '';
                const lineEnd = props.line_end ? String(props.line_end) : '';
                
                // Construct standard local file URL
                let fileUrl = 'file://' + projectRoot + '/' + srcPath;
                if (lineStart) {
                    fileUrl += '#L' + lineStart;
                }

                // Construct direct VS Code & Cursor editor deep-linking URLs
                let vscodeUrl = 'vscode://file/' + projectRoot + '/' + srcPath;
                if (lineStart) {
                    vscodeUrl += ':' + lineStart;
                }
                let cursorUrl = 'cursor://file/' + projectRoot + '/' + srcPath;
                if (lineStart) {
                    cursorUrl += ':' + lineStart;
                }

                let lineBadge = '';
                if (lineStart) {
                    if (lineEnd) {
                        lineBadge = '<span class="lineage-badge">Lines ' + lineStart + ' - ' + lineEnd + '</span>';
                    } else {
                        lineBadge = '<span class="lineage-badge">Line ' + lineStart + '</span>';
                    }
                }

                provenanceHTML = '<div class="provenance-card">' +
                    '<div class="provenance-header-row">' +
                        '<span class="relation-type" style="color: #10b981;">Code Lineage</span>' +
                        lineBadge +
                    '</div>' +
                    '<div class="provenance-link-container" style="display: flex; flex-direction: column; gap: 0.65rem;">' +
                        '<a href="' + fileUrl + '" class="provenance-link" target="_blank" title="Click to open file in browser/editor">' +
                            '<i data-lucide="file-code" style="width: 18px; height: 18px; color: #10b981; min-width: 18px;"></i>' +
                            '<span>' + escapeHtml(srcPath) + '</span>' +
                        '</a>' +
                        '<div style="display: flex; gap: 0.5rem; margin-top: 0.25rem;">' +
                            '<a href="' + vscodeUrl + '" class="btn" style="flex: 1; justify-content: center; font-size: 0.75rem; padding: 0.35rem 0.6rem; border-color: rgba(56, 189, 248, 0.25); background-color: rgba(56, 189, 248, 0.05);" title="Open directly in VS Code">' +
                                '<i data-lucide="external-link" style="width: 12px; height: 12px; color: var(--accent-color)"></i> VS Code' +
                            '</a>' +
                            '<a href="' + cursorUrl + '" class="btn" style="flex: 1; justify-content: center; font-size: 0.75rem; padding: 0.35rem 0.6rem; border-color: rgba(16, 185, 129, 0.25); background-color: rgba(16, 185, 129, 0.05);" title="Open directly in Cursor">' +
                                '<i data-lucide="external-link" style="width: 12px; height: 12px; color: #10b981"></i> Cursor' +
                            '</a>' +
                        '</div>' +
                    '</div>' +
                '</div>';
            }

            // Build properties table
            let propertiesHTML = '';
            if (Object.keys(node.properties || {}).length > 0) {
                propertiesHTML = '<table class="properties-table">';
                for (const [key, value] of Object.entries(node.properties)) {
                    propertiesHTML += '<tr><td class="properties-key">' + key + '</td><td class="properties-val">' + escapeHtml(String(value)) + '</td></tr>';
                }
                propertiesHTML += '</table>';
            } else {
                propertiesHTML = '<p style="color: var(--text-secondary); font-size: 0.875rem; font-style: italic;">No custom properties</p>';
            }

            // Find relations involving this node
            const outgoing = graphData.edges.filter(e => e.from === nodeId);
            const incoming = graphData.edges.filter(e => e.to === nodeId);

            let relationsHTML = '';
            if (outgoing.length > 0 || incoming.length > 0) {
                relationsHTML = '<div class="relations-list">';
                
                outgoing.forEach(rel => {
                    relationsHTML += '<div class="relation-item">' +
                        '<span class="relation-type">' + rel.label + ' ➔</span>' +
                        '<span class="relation-target" onclick="focusOnNode(\'' + escapeJs(rel.to) + '\')"><i data-lucide="arrow-right-circle" style="width: 14px; height: 14px; color: var(--accent-color)"></i>' + rel.to + '</span>' +
                        '</div>';
                });

                incoming.forEach(rel => {
                    relationsHTML += '<div class="relation-item">' +
                        '<span class="relation-type">➔ ' + rel.label + ' (from)</span>' +
                        '<span class="relation-target" onclick="focusOnNode(\'' + escapeJs(rel.from) + '\')"><i data-lucide="arrow-right-circle" style="width: 14px; height: 14px; color: var(--accent-color)"></i>' + rel.from + '</span>' +
                        '</div>';
                });

                relationsHTML += '</div>';
            } else {
                relationsHTML = '<p style="color: var(--text-secondary); font-size: 0.875rem; font-style: italic;">No relationships</p>';
            }

            // Set content without using any backticks in Javascript (standard concatenated single quotes)
            drawerBody.innerHTML = '<div class="entity-header">' +
                '<span class="entity-type-badge" style="background-color: ' + color.background + '; color: #0b0f19; border: 1px solid ' + color.border + '">' +
                    node.type +
                '</span>' +
                '<h2 class="entity-id">' + node.id + '</h2>' +
            '</div>' +
            (provenanceHTML ? '<div>' + provenanceHTML + '</div>' : '') +
            '<div>' +
                '<h3 class="section-title"><i data-lucide="database" style="width: 16px; height: 16px; color: var(--accent-color)"></i> Properties</h3>' +
                propertiesHTML +
            '</div>' +
            '<div>' +
                '<h3 class="section-title"><i data-lucide="git-branch" style="width: 16px; height: 16px; color: var(--accent-color)"></i> Relationships</h3>' +
                relationsHTML +
            '</div>';

            // Open the drawer
            drawer.classList.remove('collapsed');
            
            // Re-render Lucide icons inside the drawer
            lucide.createIcons();
        }

        function focusOnNode(nodeId) {
            if (!network) return;
            network.selectNodes([nodeId]);
            network.focus(nodeId, {
                scale: 1.1,
                animation: {
                    duration: 500,
                    easingFunction: "easeInOutQuad"
                }
            });
            inspectNode(nodeId);
        }

        function closeDrawer() {
            document.getElementById('inspector-drawer').classList.add('collapsed');
        }

        // Helper string escaping functions
        function escapeHtml(str) {
            return str
                .replace(/&/g, "&amp;")
                .replace(/</g, "&lt;")
                .replace(/>/g, "&gt;")
                .replace(/"/g, "&quot;")
                .replace(/'/g, "&#039;");
        }

        function escapeJs(str) {
            return str.replace(/'/g, "\\'");
        }

        // Setup Events & Control handlers
        document.getElementById('close-drawer-btn').addEventListener('click', closeDrawer);
        
        document.getElementById('refresh-btn').addEventListener('click', async () => {
            const btn = document.getElementById('refresh-btn');
            btn.disabled = true;
            btn.innerHTML = '<i data-lucide="refresh-cw" class="animate-spin" style="width: 16px; height: 16px;"></i> Synchronizing...';
            lucide.createIcons();
            
            await fetchGraphData();
            
            btn.disabled = false;
            btn.innerHTML = '<i data-lucide="refresh-cw" style="width: 16px; height: 16px;"></i> Sync Data';
            lucide.createIcons();
        });

        // Search Autocomplete / Highlight
        document.getElementById('search-input').addEventListener('input', (e) => {
            const query = e.target.value.toLowerCase().trim();
            if (!query) return;

            // Find matching node ID
            const match = graphData.nodes.find(n => n.id.toLowerCase().includes(query));
            if (match) {
                focusOnNode(match.id);
            }
        });

        // Physics Toggle
        document.getElementById('physics-toggle').addEventListener('change', (e) => {
            if (!network) return;
            network.setOptions({
                physics: { enabled: e.target.checked }
            });
        });

        // Kick off!
        (async () => {
            await fetchGraphData();
            initNetwork();
        })();
    </script>
</body>
</html>`
