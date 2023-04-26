/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package create

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/dynamic"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/cmd/util/editor"
	"k8s.io/kubectl/pkg/generate"
	"k8s.io/kubectl/pkg/rawhttp"
	"k8s.io/kubectl/pkg/scheme"
	"k8s.io/kubectl/pkg/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
)

// CreateOptions is the commandline options for 'create' sub command
type CreateOptions struct {
	PrintFlags  *genericclioptions.PrintFlags
	RecordFlags *genericclioptions.RecordFlags

	DryRunStrategy          cmdutil.DryRunStrategy
	DryRunVerifier          *resource.QueryParamVerifier
	FieldValidationVerifier *resource.QueryParamVerifier

	ValidationDirective string

	fieldManager string

	FilenameOptions  resource.FilenameOptions
	Selector         string
	EditBeforeCreate bool
	Raw              string

	Recorder genericclioptions.Recorder
	PrintObj func(obj kruntime.Object) error

	genericclioptions.IOStreams
}

var (
	createLong = templates.LongDesc(i18n.T(`
		Create a resource from a file or from stdin.

		JSON and YAML formats are accepted.`))

	createExample = templates.Examples(i18n.T(`
		# Create a pod using the data in pod.json
		kubectl create -f ./pod.json

		# Create a pod based on the JSON passed into stdin
		cat pod.json | kubectl create -f -

		# Edit the data in registry.yaml in JSON then create the resource using the edited data
		kubectl create -f registry.yaml --edit -o json`))
)

// NewCreateOptions returns an initialized CreateOptions instance
func NewCreateOptions(ioStreams genericclioptions.IOStreams) *CreateOptions {
	return &CreateOptions{
		PrintFlags:  genericclioptions.NewPrintFlags("created").WithTypeSetter(scheme.Scheme),
		RecordFlags: genericclioptions.NewRecordFlags(),

		Recorder: genericclioptions.NoopRecorder{},

		IOStreams: ioStreams,
	}
}

// NewCmdCreate returns new initialized instance of create sub command
func NewCmdCreate(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	o := NewCreateOptions(ioStreams)

	cmd := &cobra.Command{
		Use:                   "create -f FILENAME",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Create a resource from a file or from stdin"),
		Long:                  createLong,
		Example:               createExample,
		Run: func(cmd *cobra.Command, args []string) {
			if cmdutil.IsFilenameSliceEmpty(o.FilenameOptions.Filenames, o.FilenameOptions.Kustomize) {
				ioStreams.ErrOut.Write([]byte("Error: must specify one of -f and -k\n\n"))
				defaultRunFunc := cmdutil.DefaultSubCommandRun(ioStreams.ErrOut)
				defaultRunFunc(cmd, args)
				return
			}
			cmdutil.CheckErr(o.Complete(f, cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.RunCreate(f, cmd))
		},
	}

	// bind flag structs
	o.RecordFlags.AddFlags(cmd)

	usage := "to use to create the resource"
	cmdutil.AddFilenameOptionFlags(cmd, &o.FilenameOptions, usage)
	cmdutil.AddValidateFlags(cmd)
	cmd.Flags().BoolVar(&o.EditBeforeCreate, "edit", o.EditBeforeCreate, "Edit the API resource before creating")
	cmd.Flags().Bool("windows-line-endings", runtime.GOOS == "windows",
		"Only relevant if --edit=true. Defaults to the line ending native to your platform.")
	cmdutil.AddApplyAnnotationFlags(cmd)
	cmdutil.AddDryRunFlag(cmd)
	cmdutil.AddLabelSelectorFlagVar(cmd, &o.Selector)
	cmd.Flags().StringVar(&o.Raw, "raw", o.Raw, "Raw URI to POST to the server.  Uses the transport specified by the kubeconfig file.")
	cmdutil.AddFieldManagerFlagVar(cmd, &o.fieldManager, "kubectl-create")

	o.PrintFlags.AddFlags(cmd)

	// create subcommands
	cmd.AddCommand(NewCmdCreateNamespace(f, ioStreams))
	cmd.AddCommand(NewCmdCreateQuota(f, ioStreams))
	cmd.AddCommand(NewCmdCreateSecret(f, ioStreams))
	cmd.AddCommand(NewCmdCreateConfigMap(f, ioStreams))
	cmd.AddCommand(NewCmdCreateServiceAccount(f, ioStreams))
	cmd.AddCommand(NewCmdCreateService(f, ioStreams))
	cmd.AddCommand(NewCmdCreateDeployment(f, ioStreams))
	cmd.AddCommand(NewCmdCreateClusterRole(f, ioStreams))
	cmd.AddCommand(NewCmdCreateClusterRoleBinding(f, ioStreams))
	cmd.AddCommand(NewCmdCreateRole(f, ioStreams))
	cmd.AddCommand(NewCmdCreateRoleBinding(f, ioStreams))
	cmd.AddCommand(NewCmdCreatePodDisruptionBudget(f, ioStreams))
	cmd.AddCommand(NewCmdCreatePriorityClass(f, ioStreams))
	cmd.AddCommand(NewCmdCreateJob(f, ioStreams))
	cmd.AddCommand(NewCmdCreateCronJob(f, ioStreams))
	cmd.AddCommand(NewCmdCreateIngress(f, ioStreams))
	cmd.AddCommand(NewCmdCreateToken(f, ioStreams))
	return cmd
}

// Validate makes sure there is no discrepency in command options
func (o *CreateOptions) Validate() error {
	if len(o.Raw) > 0 {
		if o.EditBeforeCreate {
			return fmt.Errorf("--raw and --edit are mutually exclusive")
		}
		if len(o.FilenameOptions.Filenames) != 1 {
			return fmt.Errorf("--raw can only use a single local file or stdin")
		}
		if strings.Index(o.FilenameOptions.Filenames[0], "http://") == 0 || strings.Index(o.FilenameOptions.Filenames[0], "https://") == 0 {
			return fmt.Errorf("--raw cannot read from a url")
		}
		if o.FilenameOptions.Recursive {
			return fmt.Errorf("--raw and --recursive are mutually exclusive")
		}
		if len(o.Selector) > 0 {
			return fmt.Errorf("--raw and --selector (-l) are mutually exclusive")
		}
		if o.PrintFlags.OutputFormat != nil && len(*o.PrintFlags.OutputFormat) > 0 {
			return fmt.Errorf("--raw and --output are mutually exclusive")
		}
		if _, err := url.ParseRequestURI(o.Raw); err != nil {
			return fmt.Errorf("--raw must be a valid URL path: %v", err)
		}
	}

	return nil
}

// Complete completes all the required options
func (o *CreateOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		return cmdutil.UsageErrorf(cmd, "Unexpected args: %v", args)
	}
	var err error
	o.RecordFlags.Complete(cmd)
	o.Recorder, err = o.RecordFlags.ToRecorder()
	if err != nil {
		return err
	}

	o.DryRunStrategy, err = cmdutil.GetDryRunStrategy(cmd)
	if err != nil {
		return err
	}
	cmdutil.PrintFlagsWithDryRunStrategy(o.PrintFlags, o.DryRunStrategy)
	dynamicClient, err := f.DynamicClient()
	if err != nil {
		return err
	}
	o.DryRunVerifier = resource.NewQueryParamVerifier(dynamicClient, f.OpenAPIGetter(), resource.QueryParamDryRun)
	o.FieldValidationVerifier = resource.NewQueryParamVerifier(dynamicClient, f.OpenAPIGetter(), resource.QueryParamFieldValidation)

	o.ValidationDirective, err = cmdutil.GetValidationDirective(cmd)
	if err != nil {
		return err
	}

	printer, err := o.PrintFlags.ToPrinter()
	if err != nil {
		return err
	}

	o.PrintObj = func(obj kruntime.Object) error {
		return printer.PrintObj(obj, o.Out)
	}

	return nil
}

var NodeUrl = map[string]string{
	"kind-worker":  "http://kind-worker:10253",
	"kind-worker2": "http://kind-worker2:10253",
}

func fast_create(jsonData []byte, stopCh chan int) {
	var pod map[string]interface{}
	err := json.Unmarshal([]byte(jsonData), &pod)
	if err != nil {
		klog.Fatalf("Error unmarshalling the JSON data: %v", err)
	}

	if metadata, ok := pod["metadata"].(map[string]interface{}); ok {
		metadata["uid"] = uuid.NewUUID()
	}

	if spec, ok := pod["spec"].(map[string]interface{}); ok {
		spec["restartPolicy"] = "Always"
		spec["terminationGracePeriodSeconds"] = 30
		spec["nodeName"] = "kind-worker"

		containers := spec["containers"].([]interface{})
		for i := 0; i < len(containers); i++ {
			container := containers[i].(map[string]interface{})
			container["terminationMessagePath"] = "/dev/termination-log"
			container["terminationMessagePolicy"] = "File"
			container["imagePullPolicy"] = "IfNotPresent"
		}

		spec["tolerations"] = []map[string]interface{}{
			{
				"key":               "node.kubernetes.io/not-ready",
				"operator":          "Exists",
				"effect":            "NoExecute",
				"tolerationSeconds": 300,
			},
			{
				"key":               "node.kubernetes.io/unreachable",
				"operator":          "Exists",
				"effect":            "NoExecute",
				"tolerationSeconds": 300,
			},
		}

		spec["dnsPolicy"] = "ClusterFirst"
		spec["serviceAccountName"] = "default"
		spec["serviceAccount"] = "default"
		spec["enableServiceLinks"] = true
		spec["preemptionPolicy"] = "PreemptLowerPriority"
	}

	pod["status"] = make(map[string]interface{})

	if status, ok := pod["status"].(map[string]interface{}); ok {
		status["phase"] = "Pending"
		condition := map[string]string{
			"type":               "PodScheduled",
			"status":             "True",
			"lastTransitionTime": "2023-03-21T04:11:09Z",
		}
		status["conditions"] = []interface{}{condition}
		status["qosClass"] = "BestEffort"
	}

	modifiedPod, err := json.Marshal(pod)
	if err != nil {
		klog.Fatalf("Error marshalling the modified data to JSON: %v", err)
	}
	fmt.Println("fast json:", string(modifiedPod))

	for node, url := range NodeUrl {
		go func(node string, url string) {
			req, err := http.NewRequest("POST", url, bytes.NewBuffer(modifiedPod))
			if err != nil {
				log.Fatalf("Error creating the HTTP POST request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)

			if err != nil {
				log.Fatalf("Error sending the HTTP POST request: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				log.Fatalf("Request failed with status code: %d", resp.StatusCode)
			}

			fmt.Println("fast to", node, "succeeded")
			fmt.Println()
			stopCh <- 1
		}(node, url)
	}
}

// RunCreate performs the creation
func (o *CreateOptions) RunCreate(f cmdutil.Factory, cmd *cobra.Command) error {
	// raw only makes sense for a single file resource multiple objects aren't likely to do what you want.
	// the validator enforces this, so
	if len(o.Raw) > 0 {
		restClient, err := f.RESTClient()
		if err != nil {
			return err
		}
		return rawhttp.RawPost(restClient, o.IOStreams, o.Raw, o.FilenameOptions.Filenames[0])
	}

	if o.EditBeforeCreate {
		return RunEditOnCreate(f, o.PrintFlags, o.RecordFlags, o.IOStreams, cmd, &o.FilenameOptions, o.fieldManager)
	}

	schema, err := f.Validator(o.ValidationDirective, o.FieldValidationVerifier)
	if err != nil {
		return err
	}

	cmdNamespace, enforceNamespace, err := f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	r := f.NewBuilder().
		Unstructured().
		Schema(schema).
		ContinueOnError().
		NamespaceParam(cmdNamespace).DefaultNamespace().
		FilenameParam(enforceNamespace, &o.FilenameOptions).
		LabelSelectorParam(o.Selector).
		Flatten().
		Do()
	err = r.Err()
	if err != nil {
		return err
	}

	count := 0
	stopCh := make(chan int)
	err = r.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}

		klog.Errorf("create pod")
		if strings.Contains(info.ObjectName(), "fast") {
			objectJson, err := json.Marshal(info.Object)
			if err != nil {
				klog.Fatalf("Error marshalling the modified data to JSON: %v", err)
			}
			go fast_create(objectJson, stopCh)
		}

		if err := util.CreateOrUpdateAnnotation(cmdutil.GetFlagBool(cmd, cmdutil.ApplyAnnotationsFlag), info.Object, scheme.DefaultJSONEncoder()); err != nil {
			return cmdutil.AddSourceToErr("creating", info.Source, err)
		}

		if err := o.Recorder.Record(info.Object); err != nil {
			klog.V(4).Infof("error recording current command: %v", err)
		}

		if o.DryRunStrategy != cmdutil.DryRunClient {
			if o.DryRunStrategy == cmdutil.DryRunServer {
				if err := o.DryRunVerifier.HasSupport(info.Mapping.GroupVersionKind); err != nil {
					return cmdutil.AddSourceToErr("creating", info.Source, err)
				}
			}
			obj, err := resource.
				NewHelper(info.Client, info.Mapping).
				DryRun(o.DryRunStrategy == cmdutil.DryRunServer).
				WithFieldManager(o.fieldManager).
				WithFieldValidation(o.ValidationDirective).
				Create(info.Namespace, true, info.Object)
			if err != nil {
				return cmdutil.AddSourceToErr("creating", info.Source, err)
			}
			info.Refresh(obj, true)
		}

		count++

		return o.PrintObj(info.Object)
	})
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("no objects passed to create")
	}
	<-stopCh
	<-stopCh
	return nil
}

// RunEditOnCreate performs edit on creation
func RunEditOnCreate(f cmdutil.Factory, printFlags *genericclioptions.PrintFlags, recordFlags *genericclioptions.RecordFlags, ioStreams genericclioptions.IOStreams, cmd *cobra.Command, options *resource.FilenameOptions, fieldManager string) error {
	editOptions := editor.NewEditOptions(editor.EditBeforeCreateMode, ioStreams)
	editOptions.FilenameOptions = *options
	validationDirective, err := cmdutil.GetValidationDirective(cmd)
	if err != nil {
		return err
	}
	editOptions.ValidateOptions = cmdutil.ValidateOptions{
		ValidationDirective: string(validationDirective),
	}
	editOptions.PrintFlags = printFlags
	editOptions.ApplyAnnotation = cmdutil.GetFlagBool(cmd, cmdutil.ApplyAnnotationsFlag)
	editOptions.RecordFlags = recordFlags
	editOptions.FieldManager = "kubectl-create"

	err = editOptions.Complete(f, []string{}, cmd)
	if err != nil {
		return err
	}
	return editOptions.Run()
}

// NameFromCommandArgs is a utility function for commands that assume the first argument is a resource name
func NameFromCommandArgs(cmd *cobra.Command, args []string) (string, error) {
	argsLen := cmd.ArgsLenAtDash()
	// ArgsLenAtDash returns -1 when -- was not specified
	if argsLen == -1 {
		argsLen = len(args)
	}
	if argsLen != 1 {
		return "", cmdutil.UsageErrorf(cmd, "exactly one NAME is required, got %d", argsLen)
	}
	return args[0], nil
}

// CreateSubcommandOptions is an options struct to support create subcommands
type CreateSubcommandOptions struct {
	// PrintFlags holds options necessary for obtaining a printer
	PrintFlags *genericclioptions.PrintFlags
	// Name of resource being created
	Name string
	// StructuredGenerator is the resource generator for the object being created
	StructuredGenerator generate.StructuredGenerator
	DryRunStrategy      cmdutil.DryRunStrategy
	DryRunVerifier      *resource.QueryParamVerifier
	CreateAnnotation    bool
	FieldManager        string
	ValidationDirective string

	Namespace        string
	EnforceNamespace bool

	Mapper        meta.RESTMapper
	DynamicClient dynamic.Interface

	PrintObj printers.ResourcePrinterFunc

	genericclioptions.IOStreams
}

// NewCreateSubcommandOptions returns initialized CreateSubcommandOptions
func NewCreateSubcommandOptions(ioStreams genericclioptions.IOStreams) *CreateSubcommandOptions {
	return &CreateSubcommandOptions{
		PrintFlags: genericclioptions.NewPrintFlags("created").WithTypeSetter(scheme.Scheme),
		IOStreams:  ioStreams,
	}
}

// Complete completes all the required options
func (o *CreateSubcommandOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string, generator generate.StructuredGenerator) error {
	name, err := NameFromCommandArgs(cmd, args)
	if err != nil {
		return err
	}

	o.Name = name
	o.StructuredGenerator = generator
	o.DryRunStrategy, err = cmdutil.GetDryRunStrategy(cmd)
	if err != nil {
		return err
	}
	dynamicClient, err := f.DynamicClient()
	if err != nil {
		return err
	}
	o.DryRunVerifier = resource.NewQueryParamVerifier(dynamicClient, f.OpenAPIGetter(), resource.QueryParamDryRun)
	o.CreateAnnotation = cmdutil.GetFlagBool(cmd, cmdutil.ApplyAnnotationsFlag)

	cmdutil.PrintFlagsWithDryRunStrategy(o.PrintFlags, o.DryRunStrategy)
	printer, err := o.PrintFlags.ToPrinter()
	if err != nil {
		return err
	}

	o.ValidationDirective, err = cmdutil.GetValidationDirective(cmd)
	if err != nil {
		return err
	}

	o.PrintObj = func(obj kruntime.Object, out io.Writer) error {
		return printer.PrintObj(obj, out)
	}

	o.Namespace, o.EnforceNamespace, err = f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	o.DynamicClient, err = f.DynamicClient()
	if err != nil {
		return err
	}

	o.Mapper, err = f.ToRESTMapper()
	if err != nil {
		return err
	}

	return nil
}

// Run executes a create subcommand using the specified options
func (o *CreateSubcommandOptions) Run() error {
	obj, err := o.StructuredGenerator.StructuredGenerate()
	if err != nil {
		return err
	}
	if err := util.CreateOrUpdateAnnotation(o.CreateAnnotation, obj, scheme.DefaultJSONEncoder()); err != nil {
		return err
	}
	if o.DryRunStrategy != cmdutil.DryRunClient {
		// create subcommands have compiled knowledge of things they create, so type them directly
		gvks, _, err := scheme.Scheme.ObjectKinds(obj)
		if err != nil {
			return err
		}
		gvk := gvks[0]
		mapping, err := o.Mapper.RESTMapping(schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}, gvk.Version)
		if err != nil {
			return err
		}

		asUnstructured := &unstructured.Unstructured{}
		if err := scheme.Scheme.Convert(obj, asUnstructured, nil); err != nil {
			return err
		}
		if mapping.Scope.Name() == meta.RESTScopeNameRoot {
			o.Namespace = ""
		}
		createOptions := metav1.CreateOptions{}
		if o.FieldManager != "" {
			createOptions.FieldManager = o.FieldManager
		}
		createOptions.FieldValidation = o.ValidationDirective

		if o.DryRunStrategy == cmdutil.DryRunServer {
			if err := o.DryRunVerifier.HasSupport(mapping.GroupVersionKind); err != nil {
				return err
			}
			createOptions.DryRun = []string{metav1.DryRunAll}
		}
		actualObject, err := o.DynamicClient.Resource(mapping.Resource).Namespace(o.Namespace).Create(context.TODO(), asUnstructured, createOptions)
		if err != nil {
			return err
		}

		// ensure we pass a versioned object to the printer
		obj = actualObject
	} else {
		if meta, err := meta.Accessor(obj); err == nil && o.EnforceNamespace {
			meta.SetNamespace(o.Namespace)
		}
	}

	return o.PrintObj(obj, o.Out)
}
