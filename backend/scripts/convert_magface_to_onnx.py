"""
Convert MagFace PyTorch weights to ONNX format.

Loads the official MagFace iResNet50 model from PyTorch weights and
exports it to ONNX for use with onnxruntime (see
app/services/quality_estimation_service.py).

This is a one-off conversion tool, not a runtime dependency - torch is
only needed to run this script, not to serve requests.

Usage:
    pip install torch
    python scripts/convert_magface_to_onnx.py

Requirements:
    - 'magface_iresnet50_MS1MV2_ddp_fp32.pth' in backend/models/
      (download from https://github.com/IrvingMeng/MagFace's model zoo)
"""
import os
import sys

import torch
import torch.nn as nn


# ============================================================================
# iResNet Architecture Definition (from the MagFace repository)
# ============================================================================

def conv3x3(in_planes, out_planes, stride=1, groups=1, dilation=1):
    return nn.Conv2d(
        in_planes, out_planes, kernel_size=3, stride=stride,
        padding=dilation, groups=groups, bias=False, dilation=dilation
    )


def conv1x1(in_planes, out_planes, stride=1):
    return nn.Conv2d(in_planes, out_planes, kernel_size=1, stride=stride, bias=False)


class IBasicBlock(nn.Module):
    expansion = 1

    def __init__(self, inplanes, planes, stride=1, downsample=None,
                 groups=1, base_width=64, dilation=1):
        super(IBasicBlock, self).__init__()
        if groups != 1 or base_width != 64:
            raise ValueError('BasicBlock only supports groups=1 and base_width=64')
        if dilation > 1:
            raise NotImplementedError("Dilation > 1 not supported in BasicBlock")

        self.bn1 = nn.BatchNorm2d(inplanes, eps=1e-05)
        self.conv1 = conv3x3(inplanes, planes)
        self.bn2 = nn.BatchNorm2d(planes, eps=1e-05)
        self.prelu = nn.PReLU(planes)
        self.conv2 = conv3x3(planes, planes, stride)
        self.bn3 = nn.BatchNorm2d(planes, eps=1e-05)
        self.downsample = downsample
        self.stride = stride

    def forward(self, x):
        identity = x

        out = self.bn1(x)
        out = self.conv1(out)
        out = self.bn2(out)
        out = self.prelu(out)
        out = self.conv2(out)
        out = self.bn3(out)

        if self.downsample is not None:
            identity = self.downsample(x)

        out += identity

        return out


class IResNet(nn.Module):
    fc_scale = 7 * 7

    def __init__(self, block, layers, dropout=0, num_features=512,
                 zero_init_residual=False, groups=1, width_per_group=64,
                 replace_stride_with_dilation=None):
        super(IResNet, self).__init__()

        self.inplanes = 64
        self.dilation = 1
        if replace_stride_with_dilation is None:
            replace_stride_with_dilation = [False, False, False]
        if len(replace_stride_with_dilation) != 3:
            raise ValueError(
                "replace_stride_with_dilation should be None "
                "or a 3-element tuple, got {}".format(replace_stride_with_dilation)
            )

        self.groups = groups
        self.base_width = width_per_group

        self.conv1 = nn.Conv2d(3, self.inplanes, kernel_size=3, stride=1, padding=1, bias=False)
        self.bn1 = nn.BatchNorm2d(self.inplanes, eps=1e-05)
        self.prelu = nn.PReLU(self.inplanes)

        self.layer1 = self._make_layer(block, 64, layers[0], stride=2)
        self.layer2 = self._make_layer(block, 128, layers[1], stride=2,
                                        dilate=replace_stride_with_dilation[0])
        self.layer3 = self._make_layer(block, 256, layers[2], stride=2,
                                        dilate=replace_stride_with_dilation[1])
        self.layer4 = self._make_layer(block, 512, layers[3], stride=2,
                                        dilate=replace_stride_with_dilation[2])

        self.bn2 = nn.BatchNorm2d(512 * block.expansion, eps=1e-05)
        self.dropout = nn.Dropout(p=dropout, inplace=True)
        self.fc = nn.Linear(512 * block.expansion * self.fc_scale, num_features)
        self.features = nn.BatchNorm1d(num_features, eps=1e-05)
        nn.init.constant_(self.features.weight, 1.0)
        self.features.weight.requires_grad = False

        for m in self.modules():
            if isinstance(m, nn.Conv2d):
                nn.init.kaiming_normal_(m.weight, mode='fan_out', nonlinearity='relu')
            elif isinstance(m, (nn.BatchNorm2d, nn.GroupNorm)):
                nn.init.constant_(m.weight, 1)
                nn.init.constant_(m.bias, 0)

        if zero_init_residual:
            for m in self.modules():
                if isinstance(m, IBasicBlock):
                    nn.init.constant_(m.bn3.weight, 0)

    def _make_layer(self, block, planes, blocks, stride=1, dilate=False):
        downsample = None
        previous_dilation = self.dilation

        if dilate:
            self.dilation *= stride
            stride = 1

        if stride != 1 or self.inplanes != planes * block.expansion:
            downsample = nn.Sequential(
                conv1x1(self.inplanes, planes * block.expansion, stride),
                nn.BatchNorm2d(planes * block.expansion, eps=1e-05),
            )

        layers = []
        layers.append(
            block(self.inplanes, planes, stride, downsample, self.groups,
                  self.base_width, previous_dilation)
        )
        self.inplanes = planes * block.expansion
        for _ in range(1, blocks):
            layers.append(
                block(self.inplanes, planes, groups=self.groups,
                      base_width=self.base_width, dilation=self.dilation)
            )

        return nn.Sequential(*layers)

    def forward(self, x):
        x = self.conv1(x)
        x = self.bn1(x)
        x = self.prelu(x)

        x = self.layer1(x)
        x = self.layer2(x)
        x = self.layer3(x)
        x = self.layer4(x)

        x = self.bn2(x)
        x = torch.flatten(x, 1)
        x = self.dropout(x)
        x = self.fc(x)
        x = self.features(x)

        return x


def iresnet50(num_features=512, dropout=0.0, **kwargs):
    return IResNet(IBasicBlock, [3, 4, 14, 3],
                   num_features=num_features,
                   dropout=dropout,
                   **kwargs)


# ============================================================================
# Conversion
# ============================================================================

def convert_to_onnx():
    scripts_dir = os.path.dirname(os.path.abspath(__file__))
    models_dir = os.path.join(scripts_dir, '..', 'models')

    input_pth = os.path.join(models_dir, 'magface_iresnet50_MS1MV2_ddp_fp32.pth')
    output_onnx = os.path.join(models_dir, 'magface_iresnet50.onnx')

    if not os.path.exists(input_pth):
        print(f"ERROR: Model file not found: {input_pth}")
        print("\nDownload 'magface_iresnet50_MS1MV2_ddp_fp32.pth' from the")
        print("MagFace model zoo (https://github.com/IrvingMeng/MagFace)")
        print(f"and place it in: {models_dir}/")
        return False

    print("=" * 70)
    print("MagFace PyTorch to ONNX Conversion")
    print("=" * 70)
    print(f"Input:  {input_pth}")
    print(f"Output: {output_onnx}")
    print()

    print("Step 1/4: Initializing model architecture...")
    model = iresnet50(num_features=512, dropout=0.0)
    print("Model architecture created (iResNet50)")

    print("\nStep 2/4: Loading pretrained weights...")
    try:
        checkpoint = torch.load(input_pth, map_location='cpu')

        if isinstance(checkpoint, dict):
            if 'state_dict' in checkpoint:
                state_dict = checkpoint['state_dict']
            elif 'model' in checkpoint:
                state_dict = checkpoint['model']
            else:
                state_dict = checkpoint
        else:
            state_dict = checkpoint

        # Strip DataParallel/DistributedDataParallel wrapper prefixes and
        # drop parallel_fc (training-only head, not needed for inference).
        clean_state_dict = {}
        for key, value in state_dict.items():
            if key.startswith('parallel_fc'):
                continue

            if key.startswith('features.module.'):
                clean_key = key[len('features.module.'):]
            elif key.startswith('module.'):
                clean_key = key[len('module.'):]
            else:
                clean_key = key
            clean_state_dict[clean_key] = value

        model.load_state_dict(clean_state_dict, strict=True)
        print("Weights loaded successfully")

    except Exception as e:
        print(f"Error loading weights: {e}")
        return False

    model.eval()

    print("\nStep 3/4: Preparing for ONNX export...")
    dummy_input = torch.randn(1, 3, 112, 112)
    print("Dummy input created (1, 3, 112, 112)")

    print("\nStep 4/4: Exporting to ONNX...")
    try:
        with torch.no_grad():
            torch.onnx.export(
                model,
                dummy_input,
                output_onnx,
                input_names=['input'],
                output_names=['output'],
                dynamic_axes={
                    'input': {0: 'batch_size'},
                    'output': {0: 'batch_size'}
                },
                opset_version=14,
                do_constant_folding=True,
                export_params=True,
                verbose=False,
                dynamo=False,
            )
        print("ONNX model exported successfully")

    except Exception as e:
        print(f"Error during export: {e}")
        return False

    if os.path.exists(output_onnx):
        file_size_mb = os.path.getsize(output_onnx) / (1024 * 1024)
        print(f"\nConversion complete! File size: {file_size_mb:.1f} MB")
        print(f"Model saved to: {output_onnx}")
        return True
    else:
        print("\nOutput file was not created")
        return False


if __name__ == "__main__":
    print()
    success = convert_to_onnx()
    print()

    if success:
        print("=" * 70)
        print("Done. Quality scoring will pick up the model automatically on")
        print("next server start (FACE_QUALITY_MODEL_PATH in .env, if set,")
        print("must point at the same file).")
        print("=" * 70)
        sys.exit(0)
    else:
        print("=" * 70)
        print("Conversion failed. Please check the error messages above.")
        print("=" * 70)
        sys.exit(1)
