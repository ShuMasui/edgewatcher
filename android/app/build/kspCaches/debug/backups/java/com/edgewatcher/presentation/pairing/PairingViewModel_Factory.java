package com.edgewatcher.presentation.pairing;

import com.edgewatcher.domain.model.DeviceInfo;
import com.edgewatcher.usecase.PairDeviceUseCase;
import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Provider;
import dagger.internal.QualifierMetadata;
import dagger.internal.ScopeMetadata;
import javax.annotation.processing.Generated;

@ScopeMetadata
@QualifierMetadata
@DaggerGenerated
@Generated(
    value = "dagger.internal.codegen.ComponentProcessor",
    comments = "https://dagger.dev"
)
@SuppressWarnings({
    "unchecked",
    "rawtypes",
    "KotlinInternal",
    "KotlinInternalInJava",
    "cast",
    "deprecation",
    "nullness:initialization.field.uninitialized"
})
public final class PairingViewModel_Factory implements Factory<PairingViewModel> {
  private final Provider<PairDeviceUseCase> pairDeviceProvider;

  private final Provider<DeviceInfo> deviceInfoProvider;

  private PairingViewModel_Factory(Provider<PairDeviceUseCase> pairDeviceProvider,
      Provider<DeviceInfo> deviceInfoProvider) {
    this.pairDeviceProvider = pairDeviceProvider;
    this.deviceInfoProvider = deviceInfoProvider;
  }

  @Override
  public PairingViewModel get() {
    return newInstance(pairDeviceProvider.get(), deviceInfoProvider.get());
  }

  public static PairingViewModel_Factory create(Provider<PairDeviceUseCase> pairDeviceProvider,
      Provider<DeviceInfo> deviceInfoProvider) {
    return new PairingViewModel_Factory(pairDeviceProvider, deviceInfoProvider);
  }

  public static PairingViewModel newInstance(PairDeviceUseCase pairDevice, DeviceInfo deviceInfo) {
    return new PairingViewModel(pairDevice, deviceInfo);
  }
}
