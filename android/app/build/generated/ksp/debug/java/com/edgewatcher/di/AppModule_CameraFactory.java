package com.edgewatcher.di;

import com.edgewatcher.domain.port.CameraGateway;
import com.edgewatcher.infrastructure.camera.CameraXGateway;
import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Preconditions;
import dagger.internal.Provider;
import dagger.internal.QualifierMetadata;
import dagger.internal.ScopeMetadata;
import javax.annotation.processing.Generated;

@ScopeMetadata("javax.inject.Singleton")
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
public final class AppModule_CameraFactory implements Factory<CameraGateway> {
  private final Provider<CameraXGateway> gatewayProvider;

  private AppModule_CameraFactory(Provider<CameraXGateway> gatewayProvider) {
    this.gatewayProvider = gatewayProvider;
  }

  @Override
  public CameraGateway get() {
    return camera(gatewayProvider.get());
  }

  public static AppModule_CameraFactory create(Provider<CameraXGateway> gatewayProvider) {
    return new AppModule_CameraFactory(gatewayProvider);
  }

  public static CameraGateway camera(CameraXGateway gateway) {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.camera(gateway));
  }
}
