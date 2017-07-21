package main

// #include <libpe/pe.h>
// #include <libpe/hashes.h>
// #include <libpe/misc.h>
// #include <libpe/imports.h>
// #include <libpe/exports.h>
// #include <libpe/peres.h>
// #cgo LDFLAGS: -lpe -lssl -lcrypto -lm
// #cgo CFLAGS: -std=c99
import "C"
import (
	//Imports for measuring execution time of requests
	"time"

	//Imports for reading the config, logging and command line argument parsing.
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"sync"

	//Imports for serving on a socket and handling routing of incoming request.
	"encoding/json"
	"github.com/julienschmidt/httprouter"
	"net/http"

	// Import to reduce service memory footprint
	"runtime/debug"
)

type Result struct {
	Headers             Header       `json:"Headers"`
	Directories         []*Directory `json:"directories"`
	Directories_count   int          `json:"directories_count"`
	Sections            []*Section   `json:"sections"`
	Sections_count      int          `json:"sectionscount"`
	PEHashes            Hashes       `json:"PEHash"`
	Exports             []*Export    `json:"Exports"`
	Imports             []Import     `json:"Imports"`
	Resources           Resource     `json:"resources"`
	Entrophy            float32      `json:"Entrophy"`
	FPUTrick            bool         `json:"FPUtrick"`
	CPLAnalysis         int          `json:"CPLAnalysis"`         // 0 -> No Threat, 1 -> Malware, -1 -> Not a dll.
	CheckFakeEntryPoint int          `json:"CheckFakeEntrypoint"` //  0 -> Normal, 1 -> fake,  -1 -> null.
}

type Resource struct {
	ResourceDirectory []*RDT_RESOURCE_DIRECTORY `json:"RESOURCE_DIRECTORY"`
	DirectoryEntry    []*RDT_DIRECTORY_ENTRY    `json:"DIRECTORY_ENTRY"`
	DataString        []*RDT_DATA_STRING        `json:"DATA_STRING"`
	DataEntry         []*RDT_DATA_ENTRY         `json:"DATA_ENTRY"`
}

type RDT_RESOURCE_DIRECTORY struct {
	NodeType             int `json:"NodeType"`
	Characteristics      int `json:"Characteristics"`
	TimeDateStamp        int `json:"TimeDateStamp"`
	MajorVersion         int `json:"MajorVersion"`
	MinorVersion         int `json:"MinorVersion"`
	NumberOfNamedEntries int `json:"NumberOfNamedEntries"`
	NumberOfIdEntries    int `json:"NumberOfIdEntries"`
}

type RDT_DIRECTORY_ENTRY struct {
	NodeType          int `json:"NodeType"`
	NameOffset        int `json:"NameOffset"`
	NameIsString      int `json:"NameIsString"`
	OffsetIsDirectory int `json:"OffsetIsDirectory"`
	DataIsDirectory   int `json:"DataIsDirectory"`
}

type RDT_DATA_STRING struct {
	NodeType int `json"NodeType"`
	Strlen   int `json:"Strlen"`
	String   int `json:"String"`
}

type RDT_DATA_ENTRY struct {
	NodeType     int `json:"NodeType"`
	OffsetToData int `json:"OffsetToData"`
	Size         int `json:"Size"`
	CodePage     int `json:"CodePage"`
	Reserved     int `json:"Reserved"`
}

type Import struct {
	Dllname   string   `json:"DllName"`
	Functions []string `json:"Functions"`
}

type Export struct {
	Addr         string `json:"Addr"`
	FunctionName string `json:"FunctionName"`
}

type Header struct {
	Optional OptionalHeaders `json:"Optional"`
	Dos      DosHeaders      `json:"DosHeaders"`
	Coff     CoffHeaders     `json:"CoffHeaders"`
}

type OptionalHeaders struct {
	Magic                       int `json:"Magic"`
	MajorLinkerVersion          int `json:"MajorLinkerVersion"`
	MinorLinkerVersion          int `json:"MinorLinkerVersion"`
	SizeOfCode                  int `json:"SizeOfCode"`
	SizeOfInitializedData       int `json:"SizeOfUninitializedData"`
	SizeOfUninitializedData     int `json:"SizeOfUninitializedData"`
	AddressOfEntryPoint         int `json:"AddressOfEntryPoint"`
	BaseOfCode                  int `json:"BaseOfCode"`
	ImageBase                   int `json:"ImageBase"`
	SectionAlignment            int `json:"SectionAlignment"`
	FileAlignment               int `json:"FileAlignment"`
	MajorOperatingSystemVersion int `json:"MajorOperatingSystemVersion"`
	MinorOperatingSystemVersion int `json:"MinorOperatingSystemVersion"`
	MajorImageVersion           int `json:"MajorImageVersion"`
	MinorImageVersion           int `json:"MinorImageVersion"`
	MajorSubsystemVersion       int `json:"MajorSubsystemVersion"`
	MinorSubsystemVersion       int `json:"MinorSubsystemVersion"`
	Reserved1                   int `json:Reserved1"`
	SizeOfImage                 int `json:"SizeOfImage"`
	SizeOfHeaders               int `json:"SizeOfHeaders"`
	CheckSum                    int `json:"CheckSum"`
	Subsystem                   int `json:"Subsystem"`
	DllCharacteristics          int `json:"DllCharacteristics"`
	SizeOfStackReserve          int `json:"SizeOfStackReserve"`
	SizeOfStackCommit           int `json:"SizeOfStackCommit"`
	SizeOfHeapReserve           int `json:"SizeOfHeapReserve"`
	SizeOfHeapCommit            int `json:"SizeOfHeapCommit"`
	LoaderFlags                 int `json:"LoaderFlags"`
	NumberOfRvaAndSizes         int `json:"NumberOfRvaAndSizes"`
}

type DosHeaders struct {
	Magic    int `json:"e_magic"` // Magic Number
	Cblp     int `json:"e_cblp"`
	Cp       int `json:"e_cblp"`
	Crlc     int `json:"e_crlc"`
	Cparhdr  int `json:"e_cparhdr"`
	Minalloc int `json:"e_minalloc"`
	Maxalloc int `json:"e_maxalloc"`
	Ss       int `json:"e_ss"`
	Sp       int `json:"e_sp"`
	Csum     int `json:"e_csum"`
	Ip       int `json:"e_ip"`
	Cs       int `json:"e_cs"`
	Lfarlc   int `json:"e_lfarlc"`
	Ovno     int `json:"e_ovno"`
	Res      int `json:"e_res"`
	Oemid    int `json:"e_oemid"`
	Oeminfo  int `json:"e_oeminfo"`
	Res2     int `json:"e_res2"`
	Lfanew   int `json:"e_lfanew"`
}

type CoffHeaders struct {
	Machine              string `json:"Machine"`
	NumberOfSections     string `json:"NumberOfSections"`
	TimeDateStamp        string `json:"TimeDateStamp"`
	PointerToSymbolTable string `json:"PointerToSymbolTable"`
	NumberOfSymbols      string `json:"NumberOfSymbols"`
	SizeOfOptionalHeader string `json:"SizeOfOptionalHeader"`
	Characteristics      string `json:"Characteristics"`
}

type Directory struct {
	Name           string `json:"Name"`
	VirtualAddress string `json:"VirtualAddress"`
	Size           int    `json:"Size"`
}

type Section struct {
	Name                string `json:"Name"`
	VirtualAddress      string `json:"VirtualAddress"`
	PointerToRawData    string `json:"PointerToRawData"`
	NumberOfRelocations int    `json:"NumberOfRelocations"`
	Characteristics     string `json:"Characteristics"`
	VirtualSize         int    `json"VirtualSize"`
	SizeOfRawData       int    `json:"SizeOfRawData"`
}

type Hash struct {
	Name   string `json:"Name"`
	Md5    string `json:"md5"`
	Sha1   string `json:"sha1"`
	Sha256 string `json:"sha256"`
	Ssdeep string `json:"ssdeep"`
}

type Hashes struct {
	Headers  [3]Hash `json:"Headers"` // Only 3 Headers : dos, coff, optional
	Sections []*Hash `json:"Sections"`
	FileHash Hash    `json:"PEFile"`
	Imphash string `json"Imphash"`
}

// config structs
type Metadata struct {
	Name        string
	Version     string
	Description string
	Copyright   string
	License     string
}

type Config struct {
	HTTPBinding string
}

var (
	config   *Config
	info     *log.Logger
	metadata Metadata = Metadata{
		Name:        "pemta",
		Version:     "0.5.0", //we using a boddumanohar's testing branch as dependency
		Description: "./README.md",
		Copyright:   "Copyright 2017 Holmes Group LLC",
		License:     "./LICENSE",
	}
)

func main() {
	var (
		err        error
		configPath string
	)

	// setup logging
	info = log.New(os.Stdout, "", log.Ltime|log.Lshortfile)

	// load config
	flag.StringVar(&configPath, "config", "", "Path to the configuration file")
	flag.Parse()

	config, err = load_config(configPath)
	if err != nil {
		log.Fatalln("Couldn't decode config file without errors!", err.Error())
	}

	// setup http handlers
	router := httprouter.New()
	router.GET("/analyze/", handler_analyze)
	router.GET("/", handler_info)

	info.Printf("Binding to %s\n", config.HTTPBinding)
	log.Fatal(http.ListenAndServe(config.HTTPBinding, router))
}

// Parse a configuration file into a Config structure.

func load_config(configPath string) (*Config, error) {
	config := &Config{}

	// if no path is supplied look in the current dir
	if configPath == "" {
		configPath, _ = filepath.Abs(filepath.Dir(os.Args[0]))
		configPath += "/service.conf"
	}

	cfile, _ := os.Open(configPath)
	if err := json.NewDecoder(cfile).Decode(&config); err != nil {
		return config, err
	}

	if metadata.Description != "" {
		if data, err := ioutil.ReadFile(string(metadata.Description)); err == nil {
			metadata.Description = strings.Replace(string(data), "\n", "<br>", -1)
		}
	}

	if metadata.License != "" {
		if data, err := ioutil.ReadFile(string(metadata.License)); err == nil {
			metadata.License = strings.Replace(string(data), "\n", "<br>", -1)
		}
	}

	return config, nil
}

func handler_info(f_response http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	fmt.Fprintf(f_response, `<p>%s - %s</p>
        <hr>
        <p>%s</p>
        <hr>
        <p>%s</p>
        `,
		metadata.Name,
		metadata.Version,
		metadata.Description,
		metadata.License)
}

func handler_analyze(f_response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	// ms-xy: calling FreeOSMemory manually drastically reduces the memory
	// footprint at the cost of a little bit of cpu efficiency (due to gc runs
	// after every call to handler_analyze)
	defer debug.FreeOSMemory()
	info.Println("Serving request:", request)

	start_time := time.Now()

	obj := request.URL.Query().Get("obj")
	if obj == "" {
		http.Error(f_response, "Missing argument 'obj'", 400)
		return
	}
	sample_path := "/tmp/" + obj
	if _, err := os.Stat(sample_path); os.IsNotExist(err) {
		http.NotFound(f_response, request)
		info.Printf("Error accessing sample (file: %s):", sample_path)
		info.Println(err)
		return
	}

	var err C.pe_err_e
	var ctx C.pe_ctx_t
	cstr := C.CString(sample_path)
	// defer C.free(unsafe.Pointer(cstr))

	err = C.pe_load_file(&ctx, cstr)
	if err != C.LIBPE_E_OK {
		C.pe_error_print(C.stderr, err)
		return
	}

	err = C.pe_parse(&ctx)
	if err != C.LIBPE_E_OK {
		C.pe_error_print(C.stderr, err)

		return
	}

	if !C.pe_is_pe(&ctx) {
		return
	}

	result := &Result{}
	wg := &sync.WaitGroup{}
	wg.Add(15)

	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		result = header_coff(ctx, result)
	}(wg)

go func(wg *sync.WaitGroup) {
		defer wg.Done()
		result = header_dos(ctx, result)
	}(wg)
go func(wg *sync.WaitGroup) {
		defer wg.Done()
	result = header_optional(ctx, result) //cannot support optional headers because Golang reject incompactable field allignment.
}(wg)

go func(wg *sync.WaitGroup) {
		defer wg.Done()
	result.Directories_count = header_directories_count(ctx)
}(wg)

go func(wg *sync.WaitGroup) {
		defer wg.Done()
	result = header_directories(ctx, result)
}(wg)

go func(wg *sync.WaitGroup) {
		defer wg.Done()
	result = header_sections(ctx, result)
}(wg)

go func(wg *sync.WaitGroup) {
		defer wg.Done()
	result.Sections_count = header_sections_count(ctx)
}(wg)

go func(wg *sync.WaitGroup) {
		defer wg.Done()
	result = get_hashes(ctx, result)
}(wg)

go func(wg *sync.WaitGroup) {
		defer wg.Done()
	result = get_exports(ctx, result)
}(wg)

go func(wg *sync.WaitGroup) {
		defer wg.Done()
	result = get_imports(ctx, result)
}(wg)

go func(wg *sync.WaitGroup) {
		defer wg.Done()
	result = get_resources(ctx, result)
}(wg)

go func(wg *sync.WaitGroup) {
		defer wg.Done()
	result.Entrophy = get_entrophy_file(ctx)
}(wg)

go func(wg *sync.WaitGroup) {
		defer wg.Done()
	result.FPUTrick = get_fputrick(ctx)
}(wg)

go func(wg *sync.WaitGroup) {
		defer wg.Done()
	result.CPLAnalysis = get_cpl_analysis(ctx)
}(wg)

go func(wg *sync.WaitGroup) {
		defer wg.Done()
	result.CheckFakeEntryPoint = check_fake_entrypoint(ctx)
}(wg)

wg.Wait()

	// TODO: as each of these are independent, we can use concurrency.

	f_response.Header().Set("Content-Type", "text/json; charset=utf-8")
	json2http := json.NewEncoder(f_response)

	if err := json2http.Encode(result); err != nil {
		http.Error(f_response, "Generating JSON failed", 500)
		info.Println("JSON encoding failed", err.Error())
		return
	}

	elapsed_time := time.Since(start_time)
	info.Printf("Done, total time elapsed %s.\n", elapsed_time)
}

func get_resources(ctx C.pe_ctx_t, temp_result *Result) *Result {

	resources_count := C.get_resources_count(&ctx)
	resources := C.get_resources(&ctx)

	res_count := int(resources_count.resourcesDirectory)
	dirEntry_count := int(resources_count.directoryEntry)
	dataString_count := int(resources_count.dataString)
	dataEntry_count := int(resources_count.dataEntry)

	temp_result.Resources.ResourceDirectory = make([]*RDT_RESOURCE_DIRECTORY, res_count)
	temp_result.Resources.DirectoryEntry = make([]*RDT_DIRECTORY_ENTRY, dirEntry_count)
	temp_result.Resources.DataString = make([]*RDT_DATA_STRING, dataString_count)
	temp_result.Resources.DataEntry = make([]*RDT_DATA_ENTRY, dataEntry_count)

	resourcesDirectory := (*[1 << 30](C.type_RDT_RESOURCE_DIRECTORY))(unsafe.Pointer(resources.resourcesDirectory))[:res_count:res_count]
	for i := 0; i < res_count; i++ {
		temp_result.Resources.ResourceDirectory[i] = &RDT_RESOURCE_DIRECTORY{
			NodeType:             int(resourcesDirectory[i].NodeType),
			Characteristics:      int(resourcesDirectory[i].Characteristics),
			TimeDateStamp:        int(resourcesDirectory[i].TimeDateStamp),
			MajorVersion:         int(resourcesDirectory[i].MajorVersion),
			MinorVersion:         int(resourcesDirectory[i].MinorVersion),
			NumberOfNamedEntries: int(resourcesDirectory[i].NumberOfNamedEntries),
			NumberOfIdEntries:    int(resourcesDirectory[i].NumberOfIdEntries),
		}
	}

	directoryEntry := (*[1 << 30](C.type_RDT_DIRECTORY_ENTRY))(unsafe.Pointer(resources.directoryEntry))[:dirEntry_count:dirEntry_count]
	for i := 0; i < dirEntry_count; i++ {
		temp_result.Resources.DirectoryEntry[i] = &RDT_DIRECTORY_ENTRY{
			NodeType:          int(directoryEntry[i].NodeType),
			NameOffset:      int(directoryEntry[i].NameOffset),
			NameIsString:      int(directoryEntry[i].NameIsString),
			OffsetIsDirectory: int(directoryEntry[i].OffsetIsDirectory),
			DataIsDirectory:   int(directoryEntry[i].DataIsDirectory),
		}
	}

	dataString := (*[1 << 30](C.type_RDT_DATA_STRING))(unsafe.Pointer(resources.dataString))[:dataString_count:dataString_count]
	for i := 0; i < dataString_count; i++ {
		temp_result.Resources.DataString[i] = &RDT_DATA_STRING{
			NodeType: int(dataString[i].NodeType),
			Strlen:   int(dataString[i].Strlen),
			String:   int(dataString[i].String),
		}
	}

	dataEntry := (*[1 << 30](C.type_RDT_DATA_ENTRY))(unsafe.Pointer(resources.dataEntry))[:dirEntry_count:dirEntry_count]
	for i := 0; i < dataEntry_count; i++ {
		temp_result.Resources.DataEntry[i] = &RDT_DATA_ENTRY{
			NodeType:     int(dataEntry[i].NodeType),
			OffsetToData: int(dataEntry[i].OffsetToData),
			Size:         int(dataEntry[i].Size),
			CodePage:     int(dataEntry[i].CodePage),
			Reserved:     int(dataEntry[i].Reserved),
		}
	}
	return temp_result
}
func get_imports(ctx C.pe_ctx_t, temp_result *Result) *Result {
	imports := C.get_imports(&ctx)
	dll_count := int(imports.dll_count)
	//fmt.Printf("Dll count %d\n", dll_count)
	if dll_count == 0 {
		//fmt.Println(" Exports count 0 ")
		return temp_result
	}
	temp_result.Imports = make([]Import, dll_count)

	// converting c array into Go slices because indexing of C arrays in not possible in Go.
	dllnames := (*[1 << 30](*C.char))(unsafe.Pointer(imports.dllNames))[:dll_count:dll_count]
	for i := 0; i < dll_count; i++ {

		temp_result.Imports[i].Dllname = C.GoString(dllnames[i])

		dllname_functions := (*[1 << 30](C.function_t))(unsafe.Pointer(imports.functions))[:dll_count:dll_count]

		functions_count := int(dllname_functions[i].count)
		function_names := (*[1 << 30](*C.char))(unsafe.Pointer(dllname_functions[i].functions))[:functions_count:functions_count]

		temp_result.Imports[i].Functions = make([]string, functions_count)
		for j := 0; j < functions_count; j++ {

			temp_result.Imports[i].Functions[j] = C.GoString(function_names[j])

		}
	}
	return temp_result
}

func check_fake_entrypoint(ctx C.pe_ctx_t) int {
	fake := C.pe_has_fake_entrypoint(&ctx)
	return int(fake)
}

func get_cpl_analysis(ctx C.pe_ctx_t) int {
	cpl := C.get_cpl_analysis(&ctx)
	return int(cpl)
}

func get_fputrick(ctx C.pe_ctx_t) bool {
	detected := C.fpu_trick(&ctx)
	return bool(detected)
}
func get_entrophy_file(ctx C.pe_ctx_t) float32 {

	info.Println("calculating entrophy")
	entrophy := C.calculate_entropy_file(&ctx)

	return float32(entrophy)
}

func get_exports(ctx C.pe_ctx_t, temp_result *Result) *Result {

	exports := C.get_exports(&ctx)
	if exports == nil {
		info.Println("nill value")
		return temp_result
	}
	count := C.get_exports_functions_count(&ctx)
	length := int(count)
	if length == 0 {
		return temp_result
	}
	sliceV := (*[1 << 30](C.exports_t))(unsafe.Pointer(exports))[:length:length] // converting c array into Go slices
	temp_result.Exports = make([]*Export, length)

	for i := 0; i < length; i++ {

		temp_result.Exports[i] = &Export{
			Addr:         fmt.Sprintf("%X", sliceV[i].addr),
			FunctionName: C.GoString(sliceV[i].function_name),
		}
	}
	return temp_result
}

func header_coff(ctx C.pe_ctx_t, temp_result *Result) *Result {
	coff := C.pe_coff(&ctx)

	temp_result.Headers.Coff.Machine = fmt.Sprintf("%X", int(coff.Machine))
	temp_result.Headers.Coff.NumberOfSections = fmt.Sprintf("%X", int(coff.NumberOfSections))
	timestamp := getTimestamp(int(coff.TimeDateStamp))
	temp_result.Headers.Coff.TimeDateStamp = fmt.Sprintf("%s", timestamp)
	temp_result.Headers.Coff.PointerToSymbolTable = fmt.Sprintf("%X", int(coff.PointerToSymbolTable))
	temp_result.Headers.Coff.NumberOfSymbols = fmt.Sprintf("%X", int(coff.NumberOfSymbols))
	temp_result.Headers.Coff.SizeOfOptionalHeader = fmt.Sprintf("%X", int(coff.SizeOfOptionalHeader))
	temp_result.Headers.Coff.Characteristics = fmt.Sprintf("%X", int(coff.Characteristics))

	return temp_result
}

func header_dos(ctx C.pe_ctx_t, temp_result *Result) *Result {
	dos := C.pe_dos(&ctx)

	temp_result.Headers.Dos.Magic = int(dos.e_magic)
	temp_result.Headers.Dos.Cblp = int(dos.e_cblp)
	temp_result.Headers.Dos.Cp = int(dos.e_cp)
	temp_result.Headers.Dos.Crlc = int(dos.e_crlc)
	temp_result.Headers.Dos.Cparhdr = int(dos.e_cparhdr)
	temp_result.Headers.Dos.Minalloc = int(dos.e_minalloc)
	temp_result.Headers.Dos.Maxalloc = int(dos.e_maxalloc)
	temp_result.Headers.Dos.Ss = int(dos.e_ss)
	temp_result.Headers.Dos.Sp = int(dos.e_sp)
	temp_result.Headers.Dos.Csum = int(dos.e_csum)
	temp_result.Headers.Dos.Ip = int(dos.e_ip)
	temp_result.Headers.Dos.Cs = int(dos.e_cs)
	temp_result.Headers.Dos.Lfarlc = int(dos.e_lfarlc)
	temp_result.Headers.Dos.Ovno = int(dos.e_ovno)
	temp_result.Headers.Dos.Res = int(dos.e_res[3])
	temp_result.Headers.Dos.Oemid = int(dos.e_oemid)
	temp_result.Headers.Dos.Oeminfo = int(dos.e_oeminfo)
	temp_result.Headers.Dos.Res2 = int(dos.e_res2[9])
	temp_result.Headers.Dos.Lfanew = int(dos.e_lfanew)

	return temp_result
}

func header_optional(ctx C.pe_ctx_t, temp_result *Result) *Result {
	optional := C.pe_optional(&ctx)
	if optional._type == C.MAGIC_PE32 {
		//fmt.Println(optional._32.Magic)
		temp_result.Headers.Optional.Magic = int(optional._32.Magic)

		temp_result.Headers.Optional.MajorLinkerVersion = int(optional._32.MajorLinkerVersion)
		temp_result.Headers.Optional.MinorLinkerVersion = int(optional._32.MinorLinkerVersion)
		temp_result.Headers.Optional.SizeOfCode = int(optional._32.SizeOfCode)
		temp_result.Headers.Optional.SizeOfInitializedData = int(optional._32.SizeOfInitializedData)
		temp_result.Headers.Optional.SizeOfUninitializedData = int(optional._32.SizeOfUninitializedData)
		temp_result.Headers.Optional.AddressOfEntryPoint = int(optional._32.AddressOfEntryPoint)
		temp_result.Headers.Optional.BaseOfCode = int(optional._32.BaseOfCode)
		temp_result.Headers.Optional.ImageBase = int(optional._32.ImageBase)
		temp_result.Headers.Optional.SectionAlignment = int(optional._32.SectionAlignment)
		temp_result.Headers.Optional.FileAlignment = int(optional._32.FileAlignment)
		temp_result.Headers.Optional.MajorOperatingSystemVersion = int(optional._32.MajorOperatingSystemVersion)
		temp_result.Headers.Optional.MinorOperatingSystemVersion = int(optional._32.MinorOperatingSystemVersion)
		temp_result.Headers.Optional.MajorImageVersion = int(optional._32.MajorImageVersion)
		temp_result.Headers.Optional.MinorImageVersion = int(optional._32.MinorImageVersion)
		temp_result.Headers.Optional.MajorSubsystemVersion = int(optional._32.MajorSubsystemVersion)
		temp_result.Headers.Optional.MinorSubsystemVersion = int(optional._32.MinorSubsystemVersion)
		temp_result.Headers.Optional.Reserved1 = int(optional._32.Reserved1)
		temp_result.Headers.Optional.SizeOfImage = int(optional._32.SizeOfImage)
		temp_result.Headers.Optional.SizeOfHeaders = int(optional._32.SizeOfHeaders)
		temp_result.Headers.Optional.CheckSum = int(optional._32.CheckSum)
		temp_result.Headers.Optional.Subsystem = int(optional._32.Subsystem)
		temp_result.Headers.Optional.DllCharacteristics = int(optional._32.DllCharacteristics)
		temp_result.Headers.Optional.SizeOfStackReserve = int(optional._32.SizeOfStackReserve)
		temp_result.Headers.Optional.SizeOfStackCommit = int(optional._32.SizeOfStackCommit)
		temp_result.Headers.Optional.SizeOfHeapReserve = int(optional._32.SizeOfHeapReserve)
		temp_result.Headers.Optional.SizeOfHeapCommit = int(optional._32.SizeOfHeapCommit)
		temp_result.Headers.Optional.LoaderFlags = int(optional._32.LoaderFlags)
		temp_result.Headers.Optional.NumberOfRvaAndSizes = int(optional._32.NumberOfRvaAndSizes)
	}
	if optional._type == C.MAGIC_PE64 {
		//fmt.Println(optional._32.Magic)
		temp_result.Headers.Optional.Magic = int(optional._64.Magic)

		temp_result.Headers.Optional.MajorLinkerVersion = int(optional._64.MajorLinkerVersion)
		temp_result.Headers.Optional.MinorLinkerVersion = int(optional._64.MinorLinkerVersion)
		temp_result.Headers.Optional.SizeOfCode = int(optional._64.SizeOfCode)
		temp_result.Headers.Optional.SizeOfInitializedData = int(optional._64.SizeOfInitializedData)
		temp_result.Headers.Optional.SizeOfUninitializedData = int(optional._64.SizeOfUninitializedData)
		temp_result.Headers.Optional.AddressOfEntryPoint = int(optional._64.AddressOfEntryPoint)
		temp_result.Headers.Optional.BaseOfCode = int(optional._64.BaseOfCode)
		temp_result.Headers.Optional.ImageBase = int(optional._64.ImageBase)
		temp_result.Headers.Optional.SectionAlignment = int(optional._64.SectionAlignment)
		temp_result.Headers.Optional.FileAlignment = int(optional._64.FileAlignment)
		temp_result.Headers.Optional.MajorOperatingSystemVersion = int(optional._64.MajorOperatingSystemVersion)
		temp_result.Headers.Optional.MinorOperatingSystemVersion = int(optional._64.MinorOperatingSystemVersion)
		temp_result.Headers.Optional.MajorImageVersion = int(optional._64.MajorImageVersion)
		temp_result.Headers.Optional.MinorImageVersion = int(optional._64.MinorImageVersion)
		temp_result.Headers.Optional.MajorSubsystemVersion = int(optional._64.MajorSubsystemVersion)
		temp_result.Headers.Optional.MinorSubsystemVersion = int(optional._64.MinorSubsystemVersion)
		temp_result.Headers.Optional.Reserved1 = int(optional._64.Reserved1)
		temp_result.Headers.Optional.SizeOfImage = int(optional._64.SizeOfImage)
		temp_result.Headers.Optional.SizeOfHeaders = int(optional._64.SizeOfHeaders)
		temp_result.Headers.Optional.CheckSum = int(optional._64.CheckSum)
		temp_result.Headers.Optional.Subsystem = int(optional._64.Subsystem)
		temp_result.Headers.Optional.DllCharacteristics = int(optional._64.DllCharacteristics)
		temp_result.Headers.Optional.SizeOfStackReserve = int(optional._64.SizeOfStackReserve)
		temp_result.Headers.Optional.SizeOfStackCommit = int(optional._64.SizeOfStackCommit)
		temp_result.Headers.Optional.SizeOfHeapReserve = int(optional._64.SizeOfHeapReserve)
		temp_result.Headers.Optional.SizeOfHeapCommit = int(optional._64.SizeOfHeapCommit)
		temp_result.Headers.Optional.LoaderFlags = int(optional._64.LoaderFlags)
		temp_result.Headers.Optional.NumberOfRvaAndSizes = int(optional._64.NumberOfRvaAndSizes)
	}
	return temp_result
}

func header_directories_count(ctx C.pe_ctx_t) int {

	count := C.pe_directories_count(&ctx)
	return int(count)
}

func header_directories(ctx C.pe_ctx_t, temp_result *Result) *Result {

	count := C.pe_directories_count(&ctx)
	if int(count) == 0 {
		return &Result{} // return empty result
	}
	length := int(count)
	var directories **C.IMAGE_DATA_DIRECTORY = C.pe_directories(&ctx)
	sliceV := (*[1 << 30](*C.IMAGE_DATA_DIRECTORY))(unsafe.Pointer(directories))[:length:length]
	if directories == nil {
		return &Result{} // return empty result
	}

	temp_result.Directories = make([]*Directory, length)

	var i C.ImageDirectoryEntry = 0
	for int(i) < length {
		// fmt.Println(sliceV[i].VirtualAddress)

		temp_result.Directories[i] = &Directory{
			Name:           C.GoString(C.pe_directory_name(i)),
			VirtualAddress: fmt.Sprintf("%X", int(sliceV[i].VirtualAddress)), // returns Virutal address
			Size:           int(sliceV[i].Size),
		}
		i++
	}

	return temp_result
}

func header_sections_count(ctx C.pe_ctx_t) int {
	sections_count := C.pe_sections_count(&ctx)
	return int(sections_count)
}

func get_hashes(ctx C.pe_ctx_t, temp_result *Result) *Result {
	// File Hash
	file_hash := C.get_file_hash(&ctx)
	temp_result.PEHashes.FileHash.Name = fmt.Sprintf("%s", C.GoString(file_hash.name))
	temp_result.PEHashes.FileHash.Md5 = fmt.Sprintf("%s", C.GoString(file_hash.md5))
	temp_result.PEHashes.FileHash.Sha1 = fmt.Sprintf("%s", C.GoString(file_hash.sha1))
	temp_result.PEHashes.FileHash.Sha256 = fmt.Sprintf("%s", C.GoString(file_hash.sha256))
	temp_result.PEHashes.FileHash.Ssdeep = fmt.Sprintf("%s", C.GoString(file_hash.ssdeep))

		imphash :=	C.imphash(&ctx, 2)
		temp_result.PEHashes.Imphash = C.GoString(imphash)
		fmt.Println(C.GoString(imphash))

	// Section Hash
	var sections C.hash_section = C.get_sections_hash(&ctx)
	//count := C.pe_sections_count(&ctx)
	length := int(sections.count)
	sliceV := (*[1 << 30](C.hash_))(unsafe.Pointer(sections.sections))[:length:length] // converting c array into Go slices
	temp_result.PEHashes.Sections = make([]*Hash, length)
	for i := 0; i < length; i++ {
		temp_result.PEHashes.Sections[i] = &Hash{
			Name:   C.GoString(sliceV[i].name),
			Md5:    C.GoString(sliceV[i].md5),
			Sha1:   C.GoString(sliceV[i].sha1),
			Sha256: C.GoString(sliceV[i].sha256),
			Ssdeep: C.GoString(sliceV[i].ssdeep),
		}
	}

	// Header Hash
	headers := C.get_headers_hash(&ctx)
	//temp_result.PEHashes.Headers = make([]*Hash, 4);  // only 3 headers : dos, coff, optional

	// for Dos header
	temp_result.PEHashes.Headers[0].Name = fmt.Sprintf("%s", C.GoString(headers.dos.name))
	temp_result.PEHashes.Headers[0].Md5 = fmt.Sprintf("%s", C.GoString(headers.dos.md5))
	temp_result.PEHashes.Headers[0].Sha1 = fmt.Sprintf("%s", C.GoString(headers.dos.sha1))
	temp_result.PEHashes.Headers[0].Sha256 = fmt.Sprintf("%s", C.GoString(headers.dos.sha256))
	temp_result.PEHashes.Headers[0].Ssdeep = fmt.Sprintf("%s", C.GoString(headers.dos.ssdeep))

	// for coff Header
	temp_result.PEHashes.Headers[1].Name = fmt.Sprintf("%s", C.GoString(headers.coff.name))
	temp_result.PEHashes.Headers[1].Md5 = fmt.Sprintf("%s", C.GoString(headers.coff.md5))
	temp_result.PEHashes.Headers[1].Sha1 = fmt.Sprintf("%s", C.GoString(headers.coff.sha1))
	temp_result.PEHashes.Headers[1].Sha256 = fmt.Sprintf("%s", C.GoString(headers.coff.sha256))
	temp_result.PEHashes.Headers[1].Ssdeep = fmt.Sprintf("%s", C.GoString(headers.coff.ssdeep))

	// for Optional Header
	temp_result.PEHashes.Headers[2].Name = fmt.Sprintf("%s", C.GoString(headers.optional.name))
	temp_result.PEHashes.Headers[2].Md5 = fmt.Sprintf("%s", C.GoString(headers.optional.md5))
	temp_result.PEHashes.Headers[2].Sha1 = fmt.Sprintf("%s", C.GoString(headers.optional.sha1))
	temp_result.PEHashes.Headers[2].Sha256 = fmt.Sprintf("%s", C.GoString(headers.optional.sha256))
	temp_result.PEHashes.Headers[2].Ssdeep = fmt.Sprintf("%s", C.GoString(headers.optional.ssdeep))

	return temp_result

}
/*func union_to_guint32_ptr(cbytes [8]byte) (result *_Ctype_uint32_t) {
    buf := bytes.NewBuffer(cbytes[:])
	var ptr uint64
    if err := binary.Read(buf, binary.LittleEndian, &ptr); err == nil {
    return (*_Ctype_guint32)(unsafe.Pointer(ptr))
	}
  return nil
}*/

func header_sections(ctx C.pe_ctx_t, temp_result *Result) *Result {
	count := C.pe_sections_count(&ctx)
	if int(count) == 0 {
		return &Result{} // return empty result
	}
	length := int(count)
	var sections **C.IMAGE_SECTION_HEADER = C.pe_sections(&ctx)
	sliceV := (*[1 << 30](*C.IMAGE_SECTION_HEADER))(unsafe.Pointer(sections))[:length:length]
	if sections == nil {
		return &Result{} // return empty result
	}
	type tagKbdInput struct {
		    typ uint32
			   va  C.uint32_t
			}
	temp_result.Sections = make([]*Section, length)
	for i := 0; i < length; i++ {
		//fmt.Println(sliceV[i].VirtualAddress)
		temp_result.Sections[i] = &Section{
			Name:                fmt.Sprintf("%s", sliceV[i].Name),
			VirtualAddress:      fmt.Sprintf("%X", int(sliceV[i].VirtualAddress)),
			PointerToRawData:    fmt.Sprintf("%X", int(sliceV[i].PointerToRawData)),
			NumberOfRelocations: int(sliceV[i].NumberOfRelocations),
			Characteristics:     fmt.Sprintf("%X", int(sliceV[i].VirtualAddress)),
			//VirtualSize:          ,
			SizeOfRawData:        int(sliceV[i].SizeOfRawData),
		}
		fmt.Println("hello ")
		fmt.Println((*tagKbdInput)(unsafe.Pointer(sliceV[i])).va)
		
	}

	return temp_result

}

func getTimestamp(unixtime int) string {
	i, err := strconv.ParseInt("956165981", 10, 64)
	if err != nil {
		panic(err)
	}
	tm := time.Unix(i, 0)
	timestamp := fmt.Sprintf("%s", tm.String())
	return timestamp
}
