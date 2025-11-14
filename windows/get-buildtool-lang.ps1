# Windows equivalent of posix/get-buildtool-lang

param(
    [string]$RepoPath = "./"
)

# Define language and their corresponding extensions using hashtable
$LangMap = @{
    "Java" = @("java")
    "Python" = @("py")
    "JavaScript" = @("js")
    "TypeScript" = @("ts")
    "C" = @("c")
    "C++" = @("cpp", "cxx", "cc")
    "CSharp" = @("cs")
    "PHP" = @("php")
    "Golang" = @("go")
    "Rust" = @("rs")
    "Kotlin" = @("kt", "kts")
    "Lua" = @("lua")
    "Dart" = @("dart")
    "Ruby" = @("rb")
    "Swift" = @("swift")
    "R" = @("r")
    "VBScript" = @("vb")
    "Groovy" = @("groovy")
    "Scala" = @("scala")
    "Perl" = @("pl")
    "Godot" = @("gd")
    "Objective-C" = @("m")
    "Elixir" = @("exs")
    "Haskell" = @("hs")
    "Pascal" = @("pas")
    "Lisp" = @("lisp", "cl", "clj", "cljs")
    "Julia" = @("jl")
    "Zig" = @("zig")
    "Fortran" = @("f")
    "Solidity" = @("sol")
    "Ada" = @("adb")
    "Erlang" = @("erl", "hrl")
    "F#" = @("fs")
    "Apex" = @("cls")
    "Prolog" = @("pro")
    "OCaml" = @("ml")
    "COBOL" = @("cbl", "cob")
    "Crystal" = @("cr")
    "Nim" = @("nim")
    "Assembly" = @("asm")
    "VBA" = @("vba")
    "Shell" = @("sh", "bash")
    "PowerShell" = @("ps1")
    "SQL" = @("sql")
    "HTML" = @("html", "htm")
    "CSS" = @("css")
}

# Function to detect languages based on file extensions
function Detect-Languages {
    param([string]$Path)
    
    $DetectedLangs = @()
    $Files = Get-ChildItem -Path $Path -Recurse -File -ErrorAction SilentlyContinue | Where-Object { -not $_.Name.StartsWith('.') }
    
    foreach ($Lang in $LangMap.Keys) {
        $Extensions = $LangMap[$Lang]
        $Found = $false
        
        foreach ($Ext in $Extensions) {
            if ($Files | Where-Object { $_.Extension -eq ".$Ext" }) {
                $Found = $true
                break
            }
        }
        
        if ($Found) {
            $DetectedLangs += $Lang
        }
    }
    
    return ($DetectedLangs -join ",")
}

# Function to detect build tools
function Detect-BuildTool {
    param([string]$Path)
    
    $BuildTool = ""
    
    if (Test-Path (Join-Path $Path "pom.xml")) {
        $BuildTool = "Maven"
    } elseif (Test-Path (Join-Path $Path "build.gradle")) {
        $BuildTool = "Gradle"
    } elseif (Test-Path (Join-Path $Path "package.json")) {
        $BuildTool = "Node"
    } elseif (Test-Path (Join-Path $Path "yarn.lock")) {
        $BuildTool = "Yarn"
    } elseif (Test-Path (Join-Path $Path "go.mod")) {
        $BuildTool = "Go"
    } elseif (Test-Path (Join-Path $Path "WORKSPACE")) {
        $BuildTool = "Bazel"
    } elseif (Get-ChildItem -Path $Path -Filter "*.csproj" -ErrorAction SilentlyContinue) {
        $BuildTool = "Dotnet"
    }
    
    return $BuildTool
}

# Detect languages and build tools
$HARNESS_LANG = Detect-Languages -Path $RepoPath
$HARNESS_BUILD_TOOL = Detect-BuildTool -Path $RepoPath

# Create JSON structure (same format as posix version)
$JsonContent = @{
    harness_lang = $HARNESS_LANG
    harness_build_tool = $HARNESS_BUILD_TOOL
} | ConvertTo-Json

# Output the JSON content to the specified file
if ($env:PLUGIN_BUILD_TOOL_FILE) {
    $JsonContent | Out-File -FilePath $env:PLUGIN_BUILD_TOOL_FILE -Encoding UTF8
    Write-Host "Build tool info written to: $env:PLUGIN_BUILD_TOOL_FILE"
} else {
    Write-Host $JsonContent
}
