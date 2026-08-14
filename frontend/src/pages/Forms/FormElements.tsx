import PageBreadcrumb from "../../components/common/PageBreadCrumb";
import DefaultInputs from "../../components/form/showcase/DefaultInputs";
import InputGroup from "../../components/form/showcase/InputGroup";
import DropzoneComponent from "../../components/form/showcase/DropZone";
import CheckboxComponents from "../../components/form/showcase/CheckboxComponents";
import RadioButtons from "../../components/form/showcase/RadioButtons";
import ToggleSwitch from "../../components/form/showcase/ToggleSwitch";
import FileInputExample from "../../components/form/showcase/FileInputExample";
import SelectInputs from "../../components/form/showcase/SelectInputs";
import TextAreaInput from "../../components/form/showcase/TextAreaInput";
import InputStates from "../../components/form/showcase/InputStates";
import PageMeta from "../../components/common/PageMeta";

export default function FormElements() {
  return (
    <div>
      <PageMeta
        title="React.js Form Elements Dashboard | TailAdmin - React.js Admin Dashboard Template"
        description="This is React.js Form Elements  Dashboard page for TailAdmin - React.js Tailwind CSS Admin Dashboard Template"
      />
      <PageBreadcrumb pageTitle="Form Elements" />
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        <div className="space-y-6">
          <DefaultInputs />
          <SelectInputs />
          <TextAreaInput />
          <InputStates />
        </div>
        <div className="space-y-6">
          <InputGroup />
          <FileInputExample />
          <CheckboxComponents />
          <RadioButtons />
          <ToggleSwitch />
          <DropzoneComponent />
        </div>
      </div>
    </div>
  );
}
